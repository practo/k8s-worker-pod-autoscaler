# Beanstalkd golang client Ver 1.0

## INSTALL
	go get github.com/maxid/beanstalkd

## USAGE

### Producer
```go
package main

import (
	"fmt"
	"github.com/maxid/beanstalkd"
	"os"
	"strconv"
	"time"
)

func main() {
	queue, err := beanstalkd.Dial("127.0.0.1:11300")
	if err != nil {
		os.Exit(0)
	}
	for i := 0; i < 1000; i++ {
		queue.Put(1, 0*time.Second, 5*time.Second, []byte("test "+strconv.Itoa(i)))
		fmt.Println("test " + strconv.Itoa(i))
	}
	queue.Quit()
}
```

### Consumer
```go
package main

import (
	"fmt"
	"github.com/maxid/beanstalkd"
	"os"
)

func main() {
	queue, err := beanstalkd.Dial("127.0.0.1:11300")
	if err != nil {
		os.Exit(0)
	}
	for {
		job, err := queue.Reserve(20)
		if err != nil {
			fmt.Println(err)
			break
		}
		fmt.Println(job.Id, string(job.Data))
		queue.Delete(job.Id)
	}
	queue.Quit()
}
```

## Implemented Commands

Producer commands:

* use
* put

Worker commands:

* reserve
* delete
* release
* bury
* touch
* watch
* ignore

Other commands:

* peek
* peek-ready
* peek-delayed
* peek-buried
* kick
* kick-job
* stats-job
* stats-tube
* stats
* list-tubes
* list-tube-used
* list-tubes-watched
* quit
* pause-tube


# Release Notes
Latest release is v1.0 that contains API changes, see release notes [here](https://github.com/maxid/beanstalkd/blob/master/ReleaseNotes.txt)

## Author

* [Maxid Tseng](http://my.oschina.net/maxid/blog)
