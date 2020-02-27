// pool.go (copy from redigo pool.go)
package beanstalkd

import (
	"container/list"
	"errors"
	"sync"
	"time"
)

var (
	DEFAULT_MAX_ACTIVE = 30                   // default pool size
	DEFAULT_IDEL_TIME  = time.Second * 60 * 5 // connection will be removed after not use
)

var nowFunc = time.Now // for testing

// ErrPoolExhausted is returned from a pool connection method (Do, Send,
// Receive, Flush, Err) when the maximum number of database connections in the
// pool has been reached.
var ErrPoolExhausted = errors.New("beanstalkd: connection pool exhausted")

var (
	errPoolClosed = errors.New("beanstalkd: connection pool closed")
	errConnClosed = errors.New("beanstalkd: connection closed")
)

// beanstalkd pool
type BeanstalkdPool struct {
	// Dial is an application supplied function for creating and configuring a connection
	Dial func(tube string) (Client, error)

	// TestOnBorrow is an optional application supplied function for checking
	// the health of an idle connection before the connection is used again by
	// the application. Argument t is the time that the connection was returned
	// to the pool. If the function returns an error, then the connection is
	// closed.
	TestOnBorrow func(c Client, t time.Time) error

	// Maximum number of idle connections in the pool.
	MaxIdle int

	// Maximum number of connections allocated by the pool at a given time.
	// When zero, there is no limit on the number of connections in the pool.
	MaxActive int

	// Close connections after remaining idle for this duration. If the value
	// is zero, then idle connections are not closed. Applications should set
	// the timeout to a value less than the server's timeout.
	IdleTimeout time.Duration

	Wait bool

	// mu protects fields defined below.
	mu     sync.Mutex
	cond   *sync.Cond
	closed bool
	active int
	// Stack of idleClient with most recently used at the front.
	idle list.List
}

// idle beanstalkd connection
type idleClient struct {
	conn     Client
	creation time.Time
}

func NewBeanstalkdPool(fn func(tube string) (Client, error), maxIdle int) *BeanstalkdPool {
	return &BeanstalkdPool{Dial: fn, MaxIdle: maxIdle, MaxActive: DEFAULT_MAX_ACTIVE, IdleTimeout: DEFAULT_IDEL_TIME}
}

// Get gets a connection. The application must close the returned connection.
// This method always returns a valid connection so that applications can defer
// error handling to the first use of the connection. If there is an error
// getting an underlying connection, then the connection Err, Do, Send, Flush
// and Receive methods return that error.
func (this *BeanstalkdPool) Get(tube string) Client {
	c, err := this.get(tube)
	if err != nil {
		return errorClient{err}
	}
	return &pooledClient{pool: this, conn: c}
}

// ActiveCount returns the number of active connections in the pool.
func (this *BeanstalkdPool) ActiveCount() int {
	this.mu.Lock()
	active := this.active
	this.mu.Unlock()
	return active
}

// Close releases the resources used by the pool.
func (this *BeanstalkdPool) Close() error {
	this.mu.Lock()
	idle := this.idle
	this.idle.Init()
	this.closed = true
	this.active -= idle.Len()
	if this.cond != nil {
		this.cond.Broadcast()
	}
	this.mu.Unlock()
	for e := idle.Front(); e != nil; e = e.Next() {
		e.Value.(idleClient).conn.Quit()
	}
	return nil
}

// release decrements the active count and signals waiters. The caller must
// hold p.mu during the call.
func (p *BeanstalkdPool) release() {
	p.active -= 1
	if p.cond != nil {
		p.cond.Signal()
	}
}

// get prunes stale connections and returns a connection from the idle list or
// creates a new connection.
func (this *BeanstalkdPool) get(tube string) (Client, error) {
	this.mu.Lock()

	// Prune stale connections.

	if timeout := this.IdleTimeout; timeout > 0 {
		for i, n := 0, this.idle.Len(); i < n; i++ {
			e := this.idle.Back()
			if e == nil {
				break
			}
			ic := e.Value.(idleClient)
			if ic.creation.Add(timeout).After(nowFunc()) {
				break
			}
			this.idle.Remove(e)
			this.release()
			this.mu.Unlock()
			ic.conn.Quit()
			this.mu.Lock()
		}
	}

	for {

		// Get idle connection.

		for i, n := 0, this.idle.Len(); i < n; i++ {
			e := this.idle.Front()
			if e == nil {
				break
			}
			ic := e.Value.(idleClient)
			this.idle.Remove(e)
			test := this.TestOnBorrow
			this.mu.Unlock()
			if test == nil || test(ic.conn, ic.creation) == nil {
				return ic.conn, nil
			}
			ic.conn.Quit()
			this.mu.Lock()
			this.release()
		}

		// Check for pool closed before dialing a new connection.

		if this.closed {
			this.mu.Unlock()
			return nil, errors.New("beanstalkd: get on closed pool")
		}

		// Dial new connection if under limit.

		if this.MaxActive == 0 || this.active < this.MaxActive {
			dial := this.Dial
			this.active += 1
			this.mu.Unlock()
			c, err := dial(tube)
			if err != nil {
				this.mu.Lock()
				this.release()
				this.mu.Unlock()
				c = nil
			}
			return c, err
		}

		if !this.Wait {
			this.mu.Unlock()
			return nil, ErrPoolExhausted
		}

		if this.cond == nil {
			this.cond = sync.NewCond(&this.mu)
		}
		this.cond.Wait()
	}
}

func (this *BeanstalkdPool) put(c Client, forceClose bool) error {
	this.mu.Lock()
	if !this.closed && !forceClose {
		this.idle.PushFront(idleClient{creation: nowFunc(), conn: c})
		if this.idle.Len() > this.MaxIdle {
			c = this.idle.Remove(this.idle.Back()).(idleClient).conn
		} else {
			c = nil
		}
	}

	if c == nil {
		if this.cond != nil {
			this.cond.Signal()
		}
		this.mu.Unlock()
		return nil
	}

	this.release()
	this.mu.Unlock()
	c.Quit()
	return nil
}

//
type pooledClient struct {
	pool  *BeanstalkdPool
	conn  Client
	tube  string
	state int
}

func (this *pooledClient) Put(priority uint32, delay, ttr time.Duration, data []byte) (id uint64, err error) {
	return this.conn.Put(priority, delay, ttr, data)
}
func (this *pooledClient) Use(tube string) error { return nil }
func (this *pooledClient) Reserve(seconds int) (*BeanstalkdJob, error) {
	return this.conn.Reserve(seconds)
}
func (this *pooledClient) Delete(id uint64) error { return this.conn.Delete(id) }
func (this *pooledClient) Release(id uint64, priority uint32, delay time.Duration) error {
	return this.conn.Release(id, priority, delay)
}
func (this *pooledClient) Bury(id uint64, priority uint32) error  { return this.conn.Bury(id, priority) }
func (this *pooledClient) Touch(id uint64) error                  { return this.conn.Touch(id) }
func (this *pooledClient) Watch(tube string) (int, error)         { return 0, nil }
func (this *pooledClient) Ignore(tube string) (int, error)        { return 0, nil }
func (this *pooledClient) Peek(id uint64) (*BeanstalkdJob, error) { return this.conn.Peek(id) }
func (this *pooledClient) PeekReady() (*BeanstalkdJob, error)     { return this.conn.PeekReady() }
func (this *pooledClient) PeekDelayed() (*BeanstalkdJob, error)   { return this.conn.PeekDelayed() }
func (this *pooledClient) PeekBuried() (*BeanstalkdJob, error)    { return this.conn.PeekBuried() }
func (this *pooledClient) Kick(bound int) (int, error)            { return this.conn.Kick(bound) }
func (this *pooledClient) KickJob(id uint64) error                { return this.conn.KickJob(id) }
func (this *pooledClient) StatsJob(id uint64) (map[string]string, error) {
	return this.conn.StatsJob(id)
}
func (this *pooledClient) StatsTube(tube string) (map[string]string, error) {
	return this.conn.StatsTube(tube)
}
func (this *pooledClient) Stats() (map[string]string, error)   { return this.conn.Stats() }
func (this *pooledClient) ListTubes() ([]string, error)        { return this.conn.ListTubes() }
func (this *pooledClient) ListTubeUsed() (string, error)       { return this.conn.ListTubeUsed() }
func (this *pooledClient) ListTubesWatched() ([]string, error) { return this.conn.ListTubesWatched() }
func (this *pooledClient) Quit() error {
	conn := this.conn
	if _, ok := conn.(errorClient); !ok {
		this.conn = errorClient{errConnClosed}
		return this.pool.put(conn, false)
	}
	return nil
}
func (this *pooledClient) PauseTube(tube string, delay int) error { return nil }

//
type errorClient struct{ err error }

func (this errorClient) Put(priority uint32, delay, ttr time.Duration, data []byte) (id uint64, err error) {
	return 0, this.err
}
func (this errorClient) Use(tube string) error                       { return this.err }
func (this errorClient) Reserve(seconds int) (*BeanstalkdJob, error) { return nil, this.err }
func (this errorClient) Delete(id uint64) error                      { return this.err }
func (this errorClient) Release(id uint64, priority uint32, delay time.Duration) error {
	return this.err
}
func (this errorClient) Bury(id uint64, priority uint32) error            { return this.err }
func (this errorClient) Touch(id uint64) error                            { return this.err }
func (this errorClient) Watch(tube string) (int, error)                   { return -1, this.err }
func (this errorClient) Ignore(tube string) (int, error)                  { return -1, this.err }
func (this errorClient) Peek(id uint64) (*BeanstalkdJob, error)           { return nil, this.err }
func (this errorClient) PeekReady() (*BeanstalkdJob, error)               { return nil, this.err }
func (this errorClient) PeekDelayed() (*BeanstalkdJob, error)             { return nil, this.err }
func (this errorClient) PeekBuried() (*BeanstalkdJob, error)              { return nil, this.err }
func (this errorClient) Kick(bound int) (int, error)                      { return 0, this.err }
func (this errorClient) KickJob(id uint64) error                          { return this.err }
func (this errorClient) StatsJob(id uint64) (map[string]string, error)    { return nil, this.err }
func (this errorClient) StatsTube(tube string) (map[string]string, error) { return nil, this.err }
func (this errorClient) Stats() (map[string]string, error)                { return nil, this.err }
func (this errorClient) ListTubes() ([]string, error)                     { return nil, this.err }
func (this errorClient) ListTubeUsed() (string, error)                    { return "", this.err }
func (this errorClient) ListTubesWatched() ([]string, error)              { return nil, this.err }
func (this errorClient) Quit() error                                      { return this.err }
func (this errorClient) PauseTube(tube string, delay int) error           { return this.err }
