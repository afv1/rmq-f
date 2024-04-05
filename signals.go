package rmq

import "sync"

type Signal int32

const (
	sigStop Signal = iota
	sigSkip
	sigSleep
)

type SigChannel struct {
}

type SigChannels struct {
	mx sync.Mutex
	m  map[string]chan Signal
}

func (sigChannels *SigChannels) Add(name string, sigChanel chan Signal) {
	sigChannels.mx.Lock()
	defer sigChannels.mx.Unlock()
	sigChannels.m[name] = sigChanel
}

func (sigChannels *SigChannels) Remove(name string) {
	sigChannels.mx.Lock()
	defer sigChannels.mx.Unlock()
	delete(sigChannels.m, name)
}

func (sigChannels *SigChannels) Get(name string) chan Signal {
	return sigChannels.m[name]
}

func NewSigChannels() *SigChannels {
	return &SigChannels{
		m: make(map[string]chan Signal),
	}
}
