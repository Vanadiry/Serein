// 全局事件总线：后端统一推送错误/警告/信息到前端 SSE
package events

import (
	"encoding/json"
	"log"
	"strconv"
	"sync"
	"time"
)

type Event struct {
	ID      string `json:"id"`
	Level   string `json:"level"`
	Context string `json:"context,omitempty"`
	Message string `json:"message"`
	TS      int64  `json:"ts"`
}

type eventBus struct {
	mu          sync.Mutex
	subscribers map[chan []byte]struct{}
	history     [][]byte
	seq         int
}

// historyLimit 回放给新订阅者的历史事件条数上限
const historyLimit = 50

// eventSession 每次进程启动唯一，避免重启后 id 与旧客户端已见的重复
var eventSession = strconv.FormatInt(time.Now().UnixNano(), 36)

var globalBus = &eventBus{
	subscribers: map[chan []byte]struct{}{},
}

func Subscribe() chan []byte {
	ch := make(chan []byte, 64)
	globalBus.mu.Lock()
	defer globalBus.mu.Unlock()
	// 回放订阅前的事件（如启动期的错误/警告）
	for _, data := range globalBus.history {
		select {
		case ch <- data:
		default:
		}
	}
	globalBus.subscribers[ch] = struct{}{}
	return ch
}

func Unsubscribe(ch chan []byte) {
	globalBus.mu.Lock()
	delete(globalBus.subscribers, ch)
	close(ch)
	globalBus.mu.Unlock()
}

func Emit(level, context, message string) {
	globalBus.mu.Lock()
	defer globalBus.mu.Unlock()
	globalBus.seq++
	evt := Event{
		ID:      eventSession + "-" + strconv.Itoa(globalBus.seq),
		Level:   level,
		Context: context,
		Message: message,
		TS:      time.Now().Unix(),
	}
	data, err := json.Marshal(evt)
	if err != nil {
		log.Printf("[events] marshal error: %v", err)
		return
	}
	globalBus.history = append(globalBus.history, data)
	if len(globalBus.history) > historyLimit {
		globalBus.history = globalBus.history[len(globalBus.history)-historyLimit:]
	}
	for ch := range globalBus.subscribers {
		select {
		case ch <- data:
		default:
		}
	}
}
