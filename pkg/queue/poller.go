package queue

import (
	"time"

	"k8s.io/klog"
)

// Poller is the generic poller which manages polling of queues from
// the configured message queuing service provider
type Poller struct {
	queues         *Queues
	service        QueuingService
	threads        map[string]bool
	listThreadCh   chan chan map[string]bool
	updateThreadCh chan map[string]bool
}

func NewPoller(queues *Queues, service QueuingService) Poller {
	return Poller{
		queues:         queues,
		service:        service,
		threads:        make(map[string]bool),
		listThreadCh:   make(chan chan map[string]bool),
		updateThreadCh: make(chan map[string]bool),
	}
}

func (p *Poller) isThreadRequired(key string) bool {
	threads := p.listThreads()
	if _, ok := threads[key]; !ok {
		return false
	}
	return true
}

func (p *Poller) runPollThread(key string) {
	for {
		if !p.isThreadRequired(key) {
			return
		}
		queueSpec := p.queues.ListQueue(key)
		p.service.poll(key, queueSpec)
	}
}

func (p *Poller) updateThreads(key string, status bool) {
	p.updateThreadCh <- map[string]bool{
		key: status,
	}
}

func (p *Poller) listThreads() map[string]bool {
	listResultCh := make(chan map[string]bool)
	p.listThreadCh <- listResultCh
	return <-listResultCh
}

func (p *Poller) sync(stopCh <-chan struct{}) {
	for {
		select {
		case listResultCh := <-p.listThreadCh:
			listResultCh <- p.threads
		case threadStatus := <-p.updateThreadCh:
			for key, status := range threadStatus {
				if status == false {
					delete(p.threads, key)
					return
				}
				p.threads[key] = status
			}
		case <-stopCh:
			klog.Info("Stopping sync thread of sqs poller gracefully.")
			return
		}
	}
}

func (p *Poller) Run(stopCh <-chan struct{}) {
	go p.sync(stopCh)

	klog.Info("Starting sqs poller(s) and thread manager.")

	ticker := time.NewTicker(time.Second * 1)
	for {
		select {
		case <-ticker.C:
			queues := p.queues.List()
			// Create a new thread
			for key, _ := range queues {
				threads := p.listThreads()
				_, ok := threads[key]
				if !ok {
					p.updateThreads(key, true)
					go p.runPollThread(key)
				}
			}

			// Trigger graceful shutdown of not required threads
			for key, _ := range p.listThreads() {
				if _, ok := queues[key]; !ok {
					p.updateThreads(key, false)
				}
			}

		case <-stopCh:
			klog.Info("Stopping sqs poller(s) and thread manager gracefully.")
			return
		}
	}
}
