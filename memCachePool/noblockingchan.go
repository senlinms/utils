package memCachePool

import (
	"container/list"
	"runtime/debug"
	"sync"
	"time"
)

// ThresholdFreeOsMemory (256M) for memCache size to free to os
const (
	ThresholdFreeOsMemory = 268435456
)

var memCacheOnce sync.Once

var nbc *NoBlockingChan

// memCache Object
type noBuffferObj struct {
	b    []byte
	used int64
}

// NoBlockingChan is a no block channel for memory cache.
// the recycle time is 1 minute ;
// the recycle threshold of total memory is 268435456;
// the recycle threshold of ervry block timeout is 5 minutes
type NoBlockingChan struct {
	send      chan []byte //
	recv      chan []byte //
	freeMem   chan byte   //
	blockSize uint64      //
}

// NewNoBlockingChan for create a no blocking chan bytes with size block
func NewNoBlockingChan(blockSize ...int) *NoBlockingChan {
	memCacheOnce.Do(func() {
		nbc = &NoBlockingChan{
			send:      make(chan []byte),
			recv:      make(chan []byte),
			freeMem:   make(chan byte),
			blockSize: 1024 * 4,
		}
		go nbc.doWork()
		go nbc.freeOldMemCache()
	})
	return nbc
}

// SetBufferSize used to set no blocking channel into blockSize
func (nbc *NoBlockingChan) SetBufferSize(blockSize uint64) {
	nbc.blockSize = blockSize
}

// Very Block is 4kb
func (nbc *NoBlockingChan) makeBuffer() []byte { return make([]byte, nbc.blockSize) }

func (nbc *NoBlockingChan) bufferSize() uint64 { return 0 }

func (nbc *NoBlockingChan) doWork() {
	defer func() {
		debug.FreeOSMemory()
	}()

	items := list.New()
	for {
		if items.Len() == 0 {
			items.PushBack(noBuffferObj{
				b:    nbc.makeBuffer(),
				used: time.Now().Unix(),
			})
		}
		e := items.Front()
		select {
		case item := <-nbc.recv:
			items.PushBack(noBuffferObj{
				b:    item,
				used: time.Now().Unix(),
			})
		case nbc.send <- e.Value.(noBuffferObj).b:
			items.Remove(e)
		case <-nbc.freeMem:
			// free too old memcached
			item := items.Front()
			var freeSize uint64
			freeTime := time.Now().Unix()
			for item != nil {
				nItem := item.Next()
				if (freeTime - item.Value.(noBuffferObj).used) > 300 {
					items.Remove(item)
					item.Value = nil
				} else {
					break
				}
				item = nItem
				freeSize += nbc.blockSize
			}
			// if needed free memory more than ThresholdFreeOsMemory, call the debug.FreeOSMemory
			if freeSize > ThresholdFreeOsMemory {
				debug.FreeOSMemory()
			}
		}
	}
}

// free old memcache object, timeout = 1 minute not to be used
func (nbc *NoBlockingChan) freeOldMemCache() {
	//timeout := time.NewTimer(time.Minute * 5)
	timeout := time.NewTicker(time.Second * 60)
	for {
		select {
		case <-timeout.C:
			nbc.freeMem <- 'f'
		}
	}
}
