// client.go
package beanstalkd

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
	"bytes"
)

// beanstalkd job
type BeanstalkdJob struct {
	Id   uint64 // Job ID
	Data []byte // Job Data
}

func NewBeanstalkdJob(id uint64, data []byte) *BeanstalkdJob {
	return &BeanstalkdJob{Id: id, Data: data}
}

// Client represents a connection to a Beanstalkd server.
type Client interface {
	Put(priority uint32, delay, ttr time.Duration, data []byte) (id uint64, err error)
	Use(tube string) error
	Reserve(seconds int) (*BeanstalkdJob, error)
	Delete(id uint64) error
	Release(id uint64, priority uint32, delay time.Duration) error
	Bury(id uint64, priority uint32) error
	Touch(id uint64) error
	Watch(tube string) (int, error)
	Ignore(tube string) (int, error)
	Peek(id uint64) (*BeanstalkdJob, error)
	PeekReady() (*BeanstalkdJob, error)
	PeekDelayed() (*BeanstalkdJob, error)
	PeekBuried() (*BeanstalkdJob, error)
	Kick(bound int) (int, error)
	KickJob(id uint64) error
	StatsJob(id uint64) (map[string]string, error)
	StatsTube(tube string) (map[string]string, error)
	Stats() (map[string]string, error)
	ListTubes() ([]string, error)
	ListTubeUsed() (string, error)
	ListTubesWatched() ([]string, error)
	Quit() error
	PauseTube(tube string, delay int) error
}

var (
	space      = []byte{' '}
	crnl       = []byte{'\r', '\n'}
	yamlHead   = []byte{'-', '-', '-', '\n'}
	nl         = []byte{'\n'}
	colonSpace = []byte{':', ' '}
	minusSpace = []byte{'-', ' '}
)

var (
	errOutOfMemory    = errors.New("out of memory")
	errInternalError  = errors.New("internal error")
	errBadFormat      = errors.New("bad format")
	errUnknownCommand = errors.New("unknown command")
	errBuried         = errors.New("buried")
	errExpectedCrlf   = errors.New("expected CRLF")
	errJobTooBig      = errors.New("job too big")
	errDraining       = errors.New("draining")
	errDeadlineSoon   = errors.New("deadline soon")
	errTimedOut       = errors.New("timed out")
	errNotFound       = errors.New("not found")
)

var (
	errInvalidLen = errors.New("invalid length")
	errUnknown    = errors.New("unknown error")
)

// beanstalkd client
/*
https://github.com/kr/beanstalkd/blob/master/doc/protocol.txt

Here is a picture of the typical job lifecycle:

   put            reserve               delete
  -----> [READY] ---------> [RESERVED] --------> *poof*


Here is a picture with more possibilities:

   put with delay               release with delay
  ----------------> [DELAYED] <------------.
                        |                   |
                        | (time passes)     |
                        |                   |
   put                  v     reserve       |       delete
  -----------------> [READY] ---------> [RESERVED] --------> *poof*
                       ^  ^                |  |
                       |   \  release      |  |
                       |    `-------------'   |
                       |                      |
                       | kick                 |
                       |                      |
                       |       bury           |
                    [BURIED] <---------------'
                       |
                       |  delete
                        `--------> *poof*
*/
type BeanstalkdClient struct {
	conn   net.Conn
	addr   string
	reader *bufio.Reader
	writer *bufio.Writer
}

// dial connect to beanstalkd server
func Dial(addr string) (*BeanstalkdClient, error) {
	c, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	return &BeanstalkdClient{
		conn:   c,
		addr:   addr,
		reader: bufio.NewReader(c),
		writer: bufio.NewWriter(c),
	}, nil
}

// send
func (this *BeanstalkdClient) send(cmd string) (int, error) {
	//fmt.Println(cmd)
	data := []byte(cmd)
	n, err := this.writer.Write(data)
	if err != nil {
		return -1, err
	}
	err = this.writer.Flush()
	if err != nil {
		return -1, err
	}
	return n, nil
}

// reply
func (this *BeanstalkdClient) reply() (string, error) {
	reply, err := this.recvLine()
	if err != nil {
		return "", err
	}
	return reply, nil
}

// send and get reply
func (this *BeanstalkdClient) sendReply(cmd string) (int, string, error) {
	n, err := this.send(cmd)
	if err != nil {
		return -1, "", err
	}
	reply, err := this.reply()
	if err != nil {
		return n, "", err
	}
	return n, reply, nil
}

func (this *BeanstalkdClient) recvLine() (string, error) {
	return this.reader.ReadString('\n')
}

func (this *BeanstalkdClient) recvSlice(dataLen int) ([]byte, error) {
	buf := make([]byte, dataLen+2) // Add 2 for \r\n
	pos := 0
	for {
		n, e := this.reader.Read(buf[pos:])
		if e != nil {
			return nil, e
		}
		pos += n
		if pos >= dataLen {
			// Read all data
			break
		}
	}
	return buf, nil
}

func (this *BeanstalkdClient) recvData(data []byte) (int, error) {
	return this.reader.Read(data)
}

// parse error
func (this *BeanstalkdClient) parseError(reply string) error {
	switch reply {
	case "BURIED\r\n":
		return errBuried
	case "NOT_FOUND\r\n":
		return errNotFound
	case "OUT_OF_MEMORY\r\n":
		return errOutOfMemory
	case "INTERNAL_ERROR\r\n":
		return errInternalError
	case "BAD_FORMAT\r\n":
		return errBadFormat
	case "UNKNOWN_COMMAND\r\n":
		return errUnknownCommand
	}
	return errUnknown
}

// assert reply
func (this *BeanstalkdClient) assert(actual, expected string) error {
	if actual != expected {
		return this.parseError(actual)
	}
	return nil
}

// -------------------------
// Producer Commands
// -------------------------

/*
The "put" command is for any process that wants to insert a job into the queue.
It comprises a command line followed by the job body:

put <pri> <delay> <ttr> <bytes>\r\n
<data>\r\n

It inserts a job into the client's currently used tube (see the "use" command
below).

 - <pri> is an integer < 2**32. Jobs with smaller priority values will be
   scheduled before jobs with larger priorities. The most urgent priority is 0;
   the least urgent priority is 4,294,967,295.

 - <delay> is an integer number of seconds to wait before putting the job in
   the ready queue. The job will be in the "delayed" state during this time.

 - <ttr> -- time to run -- is an integer number of seconds to allow a worker
   to run this job. This time is counted from the moment a worker reserves
   this job. If the worker does not delete, release, or bury the job within
   <ttr> seconds, the job will time out and the server will release the job.
   The minimum ttr is 1. If the client sends 0, the server will silently
   increase the ttr to 1.

 - <bytes> is an integer indicating the size of the job body, not including the
   trailing "\r\n". This value must be less than max-job-size (default: 2**16).

 - <data> is the job body -- a sequence of bytes of length <bytes> from the
   previous line.

After sending the command line and body, the client waits for a reply, which
may be:

 - "INSERTED <id>\r\n" to indicate success.

   - <id> is the integer id of the new job

 - "BURIED <id>\r\n" if the server ran out of memory trying to grow the
   priority queue data structure.

   - <id> is the integer id of the new job

 - "EXPECTED_CRLF\r\n" The job body must be followed by a CR-LF pair, that is,
   "\r\n". These two bytes are not counted in the job size given by the client
   in the put command line.

 - "JOB_TOO_BIG\r\n" The client has requested to put a job with a body larger
   than max-job-size bytes.

 - "DRAINING\r\n" This means that the server has been put into "drain mode"
   and is no longer accepting new jobs. The client should try another server
   or disconnect and try again later.
*/
func (this *BeanstalkdClient) Put(priority uint32, delay, ttr time.Duration, data []byte) (id uint64, err error) {
	// Strip off newline chars
	// i.e. json.Encoder always appends \n
	if bytes.HasSuffix(data, []byte("\r")) {
		data = data[ :len(data)-1]
	}
	if bytes.HasSuffix(data, []byte("\n")) {
		data = data[ :len(data)-1]
	}

	cmd := fmt.Sprintf("put %d %d %d %d\r\n", priority, uint64(delay.Seconds()), uint64(ttr.Seconds()), len(data))
	cmd = cmd + string(data) + string(crnl)

	_, reply, err := this.sendReply(cmd)

	if err != nil {
		return 0, err
	}

	switch {
	case strings.Index(reply, "INSERTED") == 0:
		var id uint64
		_, perr := fmt.Sscanf(reply, "INSERTED %d\r\n", &id)
		return id, perr
	case strings.Index(reply, "BURIED") == 0:
		var id uint64
		_, perr := fmt.Sscanf(reply, "BURIED %d\r\n", &id)
		return id, perr
	case reply == "EXPECTED_CRLF\r\n":
		return 0, errExpectedCrlf
	case reply == "JOB_TOO_BIG\r\n":
		return 0, errJobTooBig
	case reply == "DRAINING\r\n":
		return 0, errDraining
	default:
		return 0, this.parseError(reply)
	}

}

/*
The "use" command is for producers. Subsequent put commands will put jobs into
the tube specified by this command. If no use command has been issued, jobs
will be put into the tube named "default".
use <tube>\r\n
 - <tube> is a name at most 200 bytes. It specifies the tube to use. If the
   tube does not exist, it will be created.
The only reply is:
USING <tube>\r\n
 - <tube> is the name of the tube now being used.
*/
func (this *BeanstalkdClient) Use(tube string) error {
	cmd := fmt.Sprintf("use %s\r\n", tube)
	_, reply, err := this.sendReply(cmd)
	if err != nil {
		return err
	}
	return this.assert(reply, fmt.Sprintf("USING %s\r\n", tube))
}

/*
A process that wants to consume jobs from the queue uses "reserve", "delete",
"release", and "bury". The first worker command, "reserve", looks like this:

reserve\r\n

Alternatively, you can specify a timeout as follows:

reserve-with-timeout <seconds>\r\n

This will return a newly-reserved job. If no job is available to be reserved,
beanstalkd will wait to send a response until one becomes available. Once a
job is reserved for the client, the client has limited time to run (TTR) the
job before the job times out. When the job times out, the server will put the
job back into the ready queue. Both the TTR and the actual time left can be
found in response to the stats-job command.

If more than one job is ready, beanstalkd will choose the one with the
smallest priority value. Within each priority, it will choose the one that
was received first.

A timeout value of 0 will cause the server to immediately return either a
response or TIMED_OUT.  A positive value of timeout will limit the amount of
time the client will block on the reserve request until a job becomes
available.

During the TTR of a reserved job, the last second is kept by the server as a
safety margin, during which the client will not be made to wait for another
job. If the client issues a reserve command during the safety margin, or if
the safety margin arrives while the client is waiting on a reserve command,
the server will respond with:

DEADLINE_SOON\r\n

This gives the client a chance to delete or release its reserved job before
the server automatically releases it.

TIMED_OUT\r\n

If a non-negative timeout was specified and the timeout exceeded before a job
became available, or if the client's connection is half-closed, the server
will respond with TIMED_OUT.

Otherwise, the only other response to this command is a successful reservation
in the form of a text line followed by the job body:

RESERVED <id> <bytes>\r\n
<data>\r\n

 - <id> is the job id -- an integer unique to this job in this instance of
   beanstalkd.

 - <bytes> is an integer indicating the size of the job body, not including
   the trailing "\r\n".

 - <data> is the job body -- a sequence of bytes of length <bytes> from the
   previous line. This is a verbatim copy of the bytes that were originally
   sent to the server in the put command for this job.
*/
func (this *BeanstalkdClient) Reserve(seconds int) (*BeanstalkdJob, error) {
	cmd := "reserve\r\n"
	if seconds > 0 {
		cmd = fmt.Sprintf("reserve-with-timeout %d\r\n", seconds)
	}

	_, reply, err := this.sendReply(cmd)

	if err != nil {
		return nil, err
	}

	var id uint64
	var dataLen int

	switch {
	case strings.Index(reply, "RESERVED") == 0:
		_, err = fmt.Sscanf(reply, "RESERVED %d %d\r\n", &id, &dataLen)
		if err != nil {
			return nil, err
		}
	case reply == "DEADLINE_SOON\r\n":
		return nil, errDeadlineSoon
	case reply == "TIMED_OUT\r\n":
		return nil, errTimedOut
	default:
		return nil, this.parseError(reply)
	}

	data, err := this.recvSlice(dataLen)
	if err != nil {
		return nil, err
	}
	data = data[0 : len(data)-2] // throw away \r\n suffix
	if len(data) != dataLen {
		return nil, errors.New(fmt.Sprintf("Job body length missmatch %d/%d", len(data), dataLen))
	}
	return NewBeanstalkdJob(id, data), nil
}

/*
The delete command removes a job from the server entirely. It is normally used
by the client when the job has successfully run to completion. A client can
delete jobs that it has reserved, ready jobs, delayed jobs, and jobs that are
buried. The delete command looks like this:

delete <id>\r\n

 - <id> is the job id to delete.

The client then waits for one line of response, which may be:

 - "DELETED\r\n" to indicate success.

 - "NOT_FOUND\r\n" if the job does not exist or is not either reserved by the
   client, ready, or buried. This could happen if the job timed out before the
   client sent the delete command.
*/
func (this *BeanstalkdClient) Delete(id uint64) error {
	cmd := fmt.Sprintf("delete %d\r\n", id)
	_, reply, _ := this.sendReply(cmd)
	return this.assert(reply, "DELETED\r\n")
}

/*
The release command puts a reserved job back into the ready queue (and marks
its state as "ready") to be run by any client. It is normally used when the job
fails because of a transitory error. It looks like this:

release <id> <pri> <delay>\r\n

 - <id> is the job id to release.

 - <pri> is a new priority to assign to the job.

 - <delay> is an integer number of seconds to wait before putting the job in
   the ready queue. The job will be in the "delayed" state during this time.

The client expects one line of response, which may be:

 - "RELEASED\r\n" to indicate success.

 - "BURIED\r\n" if the server ran out of memory trying to grow the priority
   queue data structure.

 - "NOT_FOUND\r\n" if the job does not exist or is not reserved by the client.
*/
func (this *BeanstalkdClient) Release(id uint64, priority uint32, delay time.Duration) error {
	cmd := fmt.Sprintf("release %d %d %d\r\n", id, priority, uint64(delay.Seconds()))
	_, reply, _ := this.sendReply(cmd)
	return this.assert(reply, "RELEASED\r\n")
}

/*
The bury command puts a job into the "buried" state. Buried jobs are put into a
FIFO linked list and will not be touched by the server again until a client
kicks them with the "kick" command.

The bury command looks like this:

bury <id> <pri>\r\n

 - <id> is the job id to release.

 - <pri> is a new priority to assign to the job.

There are two possible responses:

 - "BURIED\r\n" to indicate success.

 - "NOT_FOUND\r\n" if the job does not exist or is not reserved by the client.
*/
func (this *BeanstalkdClient) Bury(id uint64, priority uint32) error {
	cmd := fmt.Sprintf("bury %d %d\r\n", id, priority)
	_, reply, _ := this.sendReply(cmd)
	return this.assert(reply, "BURIED\r\n")
}

/*
The "touch" command allows a worker to request more time to work on a job.
This is useful for jobs that potentially take a long time, but you still want
the benefits of a TTR pulling a job away from an unresponsive worker.  A worker
may periodically tell the server that it's still alive and processing a job
(e.g. it may do this on DEADLINE_SOON). The command postpones the auto
release of a reserved job until TTR seconds from when the command is issued.

The touch command looks like this:

touch <id>\r\n

 - <id> is the ID of a job reserved by the current connection.

There are two possible responses:

 - "TOUCHED\r\n" to indicate success.

 - "NOT_FOUND\r\n" if the job does not exist or is not reserved by the client.
*/
func (this *BeanstalkdClient) Touch(id uint64) error {
	cmd := fmt.Sprintf("touch %d\r\n", id)
	_, reply, _ := this.sendReply(cmd)
	return this.assert(reply, "TOUCHED\r\n")
}

/*
The "watch" command adds the named tube to the watch list for the current
connection. A reserve command will take a job from any of the tubes in the
watch list. For each new connection, the watch list initially consists of one
tube, named "default".

watch <tube>\r\n

 - <tube> is a name at most 200 bytes. It specifies a tube to add to the watch
   list. If the tube doesn't exist, it will be created.

The reply is:

WATCHING <count>\r\n

 - <count> is the integer number of tubes currently in the watch list.
*/
func (this *BeanstalkdClient) Watch(tube string) (int, error) {
	cmd := fmt.Sprintf("watch %s\r\n", tube)
	_, reply, err := this.sendReply(cmd)
	if err != nil {
		return -1, err
	}
	var count int
	_, err = fmt.Sscanf(reply, "WATCHING %d\r\n", &count)
	if err != nil {
		return -1, this.parseError(reply)
	}
	return count, nil
}

/*
The "ignore" command is for consumers. It removes the named tube from the
watch list for the current connection.

ignore <tube>\r\n

The reply is one of:

 - "WATCHING <count>\r\n" to indicate success.

   - <count> is the integer number of tubes currently in the watch list.

 - "NOT_IGNORED\r\n" if the client attempts to ignore the only tube in its
   watch list.
*/
func (this *BeanstalkdClient) Ignore(tube string) (int, error) {
	cmd := fmt.Sprintf("ignore %s\r\n", tube)
	_, reply, err := this.sendReply(cmd)
	if err != nil {
		return -1, err
	}
	var count int
	_, err = fmt.Sscanf(reply, "WATCHING %d\r\n", &count)
	if err != nil {
		if reply == "NOT_IGNORED\r\n" {
			return -1, errors.New("not ignored")
		}
		return -1, this.parseError(reply)
	}
	return count, nil
}

// -------------------------
// Other Commands
// -------------------------

/*
The peek commands let the client inspect a job in the system. There are four
variations. All but the first operate only on the currently used tube.

 - "peek <id>\r\n" - return job <id>.

 - "peek-ready\r\n" - return the next ready job.

 - "peek-delayed\r\n" - return the delayed job with the shortest delay left.

 - "peek-buried\r\n" - return the next job in the list of buried jobs.

There are two possible responses, either a single line:

 - "NOT_FOUND\r\n" if the requested job doesn't exist or there are no jobs in
   the requested state.

Or a line followed by a chunk of data, if the command was successful:

FOUND <id> <bytes>\r\n
<data>\r\n

 - <id> is the job id.

 - <bytes> is an integer indicating the size of the job body, not including
   the trailing "\r\n".

 - <data> is the job body -- a sequence of bytes of length <bytes> from the
   previous line.
*/
func (this *BeanstalkdClient) Peek(id uint64) (*BeanstalkdJob, error) {
	cmd := fmt.Sprintf("peek %d\r\n", id)
	_, reply, err := this.sendReply(cmd)
	if err != nil {
		return nil, err
	}
	return this.handlePeekReply(reply)
}

func (this *BeanstalkdClient) PeekReady() (*BeanstalkdJob, error) {
	_, reply, err := this.sendReply("peek-ready\r\n")
	if err != nil {
		return nil, err
	}
	return this.handlePeekReply(reply)
}

func (this *BeanstalkdClient) PeekDelayed() (*BeanstalkdJob, error) {
	_, reply, err := this.sendReply("peek-delayed\r\n")
	if err != nil {
		return nil, err
	}
	return this.handlePeekReply(reply)
}

func (this *BeanstalkdClient) PeekBuried() (*BeanstalkdJob, error) {
	_, reply, err := this.sendReply("peek-buried\r\n")
	if err != nil {
		return nil, err
	}
	return this.handlePeekReply(reply)
}

func (this *BeanstalkdClient) handlePeekReply(reply string) (*BeanstalkdJob, error) {
	var id uint64
	var dataLen int
	_, err := fmt.Sscanf(reply, "FOUND %d %d\r\n", &id, &dataLen)
	if err != nil {
		return nil, errNotFound
	}
	data, err := this.recvSlice(dataLen)
	if err != nil {
		return nil, err
	}
	data = data[0 : len(data)-2] // throw away \r\n suffix
	if len(data) != dataLen {
		return nil, errors.New(fmt.Sprintf("Job body length missmatch %d/%d", len(data), dataLen))
	}
	return NewBeanstalkdJob(id, data), nil
}

/*
The kick command applies only to the currently used tube. It moves jobs into
the ready queue. If there are any buried jobs, it will only kick buried jobs.
Otherwise it will kick delayed jobs. It looks like:

kick <bound>\r\n

 - <bound> is an integer upper bound on the number of jobs to kick. The server
   will kick no more than <bound> jobs.

The response is of the form:

KICKED <count>\r\n

 - <count> is an integer indicating the number of jobs actually kicked.
*/
func (this *BeanstalkdClient) Kick(bound int) (int, error) {
	cmd := fmt.Sprintf("kick %d\r\n", bound)
	_, reply, err := this.sendReply(cmd)
	if err != nil {
		return 0, err
	}
	var count int
	_, err = fmt.Sscanf(reply, "KICKED %d\r\n", &count)
	if err != nil {
		return -1, err
	}
	return count, nil
}

/*
The kick-job command is a variant of kick that operates with a single job
identified by its job id. If the given job id exists and is in a buried or
delayed state, it will be moved to the ready queue of the the same tube where it
currently belongs. The syntax is:

kick-job <id>\r\n

 - <id> is the job id to kick.

The response is one of:

 - "NOT_FOUND\r\n" if the job does not exist or is not in a kickable state. This
   can also happen upon internal errors.

 - "KICKED\r\n" when the operation succeeded.
*/
func (this *BeanstalkdClient) KickJob(id uint64) error {
	cmd := fmt.Sprintf("kick-job %d\r\n", id)
	_, reply, err := this.sendReply(cmd)
	if err != nil {
		return err
	}
	if reply == "NOT_FOUND\r\n" {
		return this.parseError(reply)
	}
	return nil
}

/*
The stats-job command gives statistical information about the specified job if
it exists. Its form is:

stats-job <id>\r\n

 - <id> is a job id.

The response is one of:

 - "NOT_FOUND\r\n" if the job does not exist.

 - "OK <bytes>\r\n<data>\r\n"

   - <bytes> is the size of the following data section in bytes.

   - <data> is a sequence of bytes of length <bytes> from the previous line. It
     is a YAML file with statistical information represented a dictionary.

The stats-job data is a YAML file representing a single dictionary of strings
to scalars. It contains these keys:

 - "id" is the job id

 - "tube" is the name of the tube that contains this job

 - "state" is "ready" or "delayed" or "reserved" or "buried"

 - "pri" is the priority value set by the put, release, or bury commands.

 - "age" is the time in seconds since the put command that created this job.

 - "time-left" is the number of seconds left until the server puts this job
   into the ready queue. This number is only meaningful if the job is
   reserved or delayed. If the job is reserved and this amount of time
   elapses before its state changes, it is considered to have timed out.

 - "file" is the number of the earliest binlog file containing this job.
   If -b wasn't used, this will be 0.

 - "reserves" is the number of times this job has been reserved.

 - "timeouts" is the number of times this job has timed out during a
   reservation.

 - "releases" is the number of times a client has released this job from a
   reservation.

 - "buries" is the number of times this job has been buried.

 - "kicks" is the number of times this job has been kicked.
*/
func (this *BeanstalkdClient) StatsJob(id uint64) (map[string]string, error) {
	cmd := fmt.Sprintf("stats-job %d\r\n", id)
	_, reply, err := this.sendReply(cmd)
	if err != nil {
		return nil, err
	}
	return this.handleMapReply(reply)
}

/*
The stats-tube command gives statistical information about the specified tube
if it exists. Its form is:

stats-tube <tube>\r\n

 - <tube> is a name at most 200 bytes. Stats will be returned for this tube.

The response is one of:

 - "NOT_FOUND\r\n" if the tube does not exist.

 - "OK <bytes>\r\n<data>\r\n"

   - <bytes> is the size of the following data section in bytes.

   - <data> is a sequence of bytes of length <bytes> from the previous line. It
     is a YAML file with statistical information represented a dictionary.

The stats-tube data is a YAML file representing a single dictionary of strings
to scalars. It contains these keys:

 - "name" is the tube's name.

 - "current-jobs-urgent" is the number of ready jobs with priority < 1024 in
   this tube.

 - "current-jobs-ready" is the number of jobs in the ready queue in this tube.

 - "current-jobs-reserved" is the number of jobs reserved by all clients in
   this tube.

 - "current-jobs-delayed" is the number of delayed jobs in this tube.

 - "current-jobs-buried" is the number of buried jobs in this tube.

 - "total-jobs" is the cumulative count of jobs created in this tube in
   the current beanstalkd process.

 - "current-using" is the number of open connections that are currently
   using this tube.

 - "current-waiting" is the number of open connections that have issued a
   reserve command while watching this tube but not yet received a response.

 - "current-watching" is the number of open connections that are currently
   watching this tube.

 - "pause" is the number of seconds the tube has been paused for.

 - "cmd-delete" is the cumulative number of delete commands for this tube

 - "cmd-pause-tube" is the cumulative number of pause-tube commands for this
   tube.

 - "pause-time-left" is the number of seconds until the tube is un-paused.
*/
func (this *BeanstalkdClient) StatsTube(tube string) (map[string]string, error) {
	cmd := fmt.Sprintf("stats-tube %s\r\n", tube)
	_, reply, err := this.sendReply(cmd)
	if err != nil {
		return nil, err
	}
	return this.handleMapReply(reply)
}

/*
The stats command gives statistical information about the system as a whole.
Its form is:

stats\r\n

The server will respond:

OK <bytes>\r\n
<data>\r\n

 - <bytes> is the size of the following data section in bytes.

 - <data> is a sequence of bytes of length <bytes> from the previous line. It
   is a YAML file with statistical information represented a dictionary.

The stats data for the system is a YAML file representing a single dictionary
of strings to scalars. Entries described as "cumulative" are reset when the
beanstalkd process starts; they are not stored on disk with the -b flag.

 - "current-jobs-urgent" is the number of ready jobs with priority < 1024.

 - "current-jobs-ready" is the number of jobs in the ready queue.

 - "current-jobs-reserved" is the number of jobs reserved by all clients.

 - "current-jobs-delayed" is the number of delayed jobs.

 - "current-jobs-buried" is the number of buried jobs.

 - "cmd-put" is the cumulative number of put commands.

 - "cmd-peek" is the cumulative number of peek commands.

 - "cmd-peek-ready" is the cumulative number of peek-ready commands.

 - "cmd-peek-delayed" is the cumulative number of peek-delayed commands.

 - "cmd-peek-buried" is the cumulative number of peek-buried commands.

 - "cmd-reserve" is the cumulative number of reserve commands.

 - "cmd-use" is the cumulative number of use commands.

 - "cmd-watch" is the cumulative number of watch commands.

 - "cmd-ignore" is the cumulative number of ignore commands.

 - "cmd-delete" is the cumulative number of delete commands.

 - "cmd-release" is the cumulative number of release commands.

 - "cmd-bury" is the cumulative number of bury commands.

 - "cmd-kick" is the cumulative number of kick commands.

 - "cmd-stats" is the cumulative number of stats commands.

 - "cmd-stats-job" is the cumulative number of stats-job commands.

 - "cmd-stats-tube" is the cumulative number of stats-tube commands.

 - "cmd-list-tubes" is the cumulative number of list-tubes commands.

 - "cmd-list-tube-used" is the cumulative number of list-tube-used commands.

 - "cmd-list-tubes-watched" is the cumulative number of list-tubes-watched
   commands.

 - "cmd-pause-tube" is the cumulative number of pause-tube commands.

 - "job-timeouts" is the cumulative count of times a job has timed out.

 - "total-jobs" is the cumulative count of jobs created.

 - "max-job-size" is the maximum number of bytes in a job.

 - "current-tubes" is the number of currently-existing tubes.

 - "current-connections" is the number of currently open connections.

 - "current-producers" is the number of open connections that have each
   issued at least one put command.

 - "current-workers" is the number of open connections that have each issued
   at least one reserve command.

 - "current-waiting" is the number of open connections that have issued a
   reserve command but not yet received a response.

 - "total-connections" is the cumulative count of connections.

 - "pid" is the process id of the server.

 - "version" is the version string of the server.

 - "rusage-utime" is the cumulative user CPU time of this process in seconds
   and microseconds.

 - "rusage-stime" is the cumulative system CPU time of this process in
   seconds and microseconds.

 - "uptime" is the number of seconds since this server process started running.

 - "binlog-oldest-index" is the index of the oldest binlog file needed to
   store the current jobs.

 - "binlog-current-index" is the index of the current binlog file being
   written to. If binlog is not active this value will be 0.

 - "binlog-max-size" is the maximum size in bytes a binlog file is allowed
   to get before a new binlog file is opened.

 - "binlog-records-written" is the cumulative number of records written
   to the binlog.

 - "binlog-records-migrated" is the cumulative number of records written
   as part of compaction.

 - "id" is a random id string for this server process, generated when each
   beanstalkd process starts.

 - "hostname" the hostname of the machine as determined by uname.
*/
func (this *BeanstalkdClient) Stats() (map[string]string, error) {
	_, reply, err := this.sendReply("stats\r\n")
	if err != nil {
		return nil, err
	}
	return this.handleMapReply(reply)
}

func (this *BeanstalkdClient) handleMapReply(reply string) (map[string]string, error) {
	var dataLen int
	switch {
	case strings.Index(reply, "OK") == 0:
		_, err := fmt.Sscanf(reply, "OK %d\r\n", &dataLen)
		if err != nil {
			return nil, err
		}
	case reply == "NOT_FOUND\r\n":
		return nil, errNotFound
	default:
		return nil, this.parseError(reply)
	}
	data := make([]byte, dataLen+2) // Add 2 for the trailing \r\n
	_, err := this.recvData(data)
	if err != nil {
		return nil, err
	}
	actual := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Index(line, ":") != -1 {
			pair := strings.Split(line, ":")
			actual[pair[0]] = strings.TrimSpace(pair[1])
		}
	}
	return actual, nil
}

/*
The list-tubes command returns a list of all existing tubes. Its form is:

list-tubes\r\n

The response is:

OK <bytes>\r\n
<data>\r\n

 - <bytes> is the size of the following data section in bytes.

 - <data> is a sequence of bytes of length <bytes> from the previous line. It
   is a YAML file containing all tube names as a list of strings.
*/
func (this *BeanstalkdClient) ListTubes() ([]string, error) {
	_, reply, err := this.sendReply("list-tubes\r\n")
	if err != nil {
		return nil, err
	}
	return this.handleListReply(reply)
}

/*
The list-tube-used command returns the tube currently being used by the
client. Its form is:

list-tube-used\r\n

The response is:

USING <tube>\r\n

 - <tube> is the name of the tube being used.
*/
func (this *BeanstalkdClient) ListTubeUsed() (string, error) {
	_, reply, err := this.sendReply("list-tube-used\r\n")
	if err != nil {
		return "", err
	}
	var tube string
	_, err = fmt.Sscanf(reply, "USING %s\r\n", &tube)
	if err != nil {
		return "", errors.New(reply)
	}
	return tube, nil
}

/*
The list-tubes-watched command returns a list tubes currently being watched by
the client. Its form is:

list-tubes-watched\r\n

The response is:

OK <bytes>\r\n
<data>\r\n

 - <bytes> is the size of the following data section in bytes.

 - <data> is a sequence of bytes of length <bytes> from the previous line. It
   is a YAML file containing watched tube names as a list of strings.
*/
func (this *BeanstalkdClient) ListTubesWatched() ([]string, error) {
	_, reply, err := this.sendReply("list-tubes-watched\r\n")
	if err != nil {
		return nil, err
	}
	return this.handleListReply(reply)
}

func (this *BeanstalkdClient) handleListReply(reply string) ([]string, error) {
	var dataLen int
	_, err := fmt.Sscanf(reply, "OK %d\r\n", &dataLen)
	if err != nil {
		return nil, err
	}
	data := make([]byte, dataLen+2) // Add 2 for the trailing \r\n
	_, err = this.recvData(data)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	tubes := make([]string, 0)
	for _, line := range lines[1 : len(lines)-2] {
		tube := strings.TrimSpace(line)
		tube = strings.TrimLeft(tube, "- ")
		tubes = append(tubes, tube)
	}
	return tubes, nil
}

/*
The quit command simply closes the connection. Its form is:

quit\r\n
*/
func (this *BeanstalkdClient) Quit() error {
	_, err := this.send("quit\r\n")
	if err != nil {
		return err
	}
	return this.conn.Close()
}

/*
The pause-tube command can delay any new job being reserved for a given time. Its form is:

pause-tube <tube-name> <delay>\r\n

 - <tube> is the tube to pause

 - <delay> is an integer number of seconds to wait before reserving any more
   jobs from the queue

There are two possible responses:

 - "PAUSED\r\n" to indicate success.

 - "NOT_FOUND\r\n" if the tube does not exist.
*/
func (this *BeanstalkdClient) PauseTube(tube string, delay int) error {
	cmd := fmt.Sprintf("pause-tube %s %d\r\n", tube, delay)
	_, reply, err := this.sendReply(cmd)
	if err != nil {
		return err
	}
	_, err = fmt.Sscanf(reply, "PAUSED\r\n")
	if err != nil {
		return errors.New(reply)
	}
	return nil
}
