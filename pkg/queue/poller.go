package queue

import (
	"time"

	"k8s.io/klog"
)

// Poller is the generic poller which manages polling of queues from
// the configured message queuing service provider
type Poller struct {
	queues         *Queues
	queueService   QueuingService
	threads        map[string]bool
	listThreadCh   chan chan map[string]bool
	updateThreadCh chan map[string]bool
}

func NewPoller(queues *Queues, queueService QueuingService) *Poller {
	return &Poller{
		queues:         queues,
		queueService:   queueService,
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
	return threads[key]
}

func (p *Poller) runPollThread(key string) {
	for {
		if !p.isThreadRequired(key) {
			return
		}
		queueSpec := p.queues.ListQueue(key)
		if queueSpec.name == "" {
			return
		}
		p.queueService.poll(key, queueSpec)
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

func (p *Poller) Sync(stopCh <-chan struct{}) {
	for {
		select {
		case listResultCh := <-p.listThreadCh:
			listResultCh <- DeepCopyThread(p.threads)
		case threadStatus := <-p.updateThreadCh:
			for key, status := range threadStatus {
				if status == false {
					delete(p.threads, key)
				}
				p.threads[key] = status
			}
		case <-stopCh:
			klog.Info("Stopping sync thread of poller gracefully.")
			return
		}
	}
}

func (p *Poller) Run(stopCh <-chan struct{}) {
	ticker := time.NewTicker(time.Second * 1)
	queueServiceName := p.queueService.GetName()
	for {
		select {
		case <-ticker.C:
			queues := p.queues.List(queueServiceName)
			// Create a new thread
			for key, _ := range queues {
				threads := p.listThreads()
				if _, ok := threads[key]; !ok {
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
			klog.Info("Stopping poller(s) and thread manager gracefully.")
			return
		}
	}
}

func DeepCopyThread(original map[string]bool) map[string]bool {
	copy := make(map[string]bool)
	for key, value := range original {
		copy[key] = value
	}
	return copy
}
