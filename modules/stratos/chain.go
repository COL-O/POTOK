package stratos

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
)

// Block — один прецедент в цепочке stratos: «ситуация → действие → результат»,
// связанный хешем с предыдущим блоком. Историю нельзя переписать незаметно.
type Block struct {
	Index    uint64 `json:"index"`
	PrevHash string `json:"prev_hash"`
	Topic    string `json:"topic"`
	Source   string `json:"source"`
	Payload  any    `json:"payload"`
	Hash     string `json:"hash"`
}

// Bytes — JSON-представление блока для store.
func (b Block) Bytes() []byte {
	out, _ := json.Marshal(b)
	return out
}

// Chain — append-only цепочка блоков с индексом по темам.
// Хеш считается от (prevHash + topic + source + payload): в скелете автора
// payload в хеш не входил, из-за чего разные прецеденты одной темы давали
// один ключ в store и затирали друг друга — здесь это исправлено.
type Chain struct {
	mu     sync.Mutex
	blocks []Block
	topic  map[string][]Block
}

// NewChain — пустая цепочка.
func NewChain() *Chain {
	return &Chain{topic: map[string][]Block{}}
}

// Append — добавить прецедент в конец цепочки.
func (c *Chain) Append(topic, source string, payload any) Block {
	c.mu.Lock()
	defer c.mu.Unlock()
	prev := ""
	if len(c.blocks) > 0 {
		prev = c.blocks[len(c.blocks)-1].Hash
	}
	b := Block{Index: uint64(len(c.blocks)), PrevHash: prev, Topic: topic, Source: source, Payload: payload}
	payloadBytes, _ := json.Marshal(payload)
	h := sha256.New()
	h.Write([]byte(prev + topic + source))
	h.Write(payloadBytes)
	b.Hash = hex.EncodeToString(h.Sum(nil))
	c.blocks = append(c.blocks, b)
	c.topic[topic] = append(c.topic[topic], b)
	return b
}

// ByTopic — все прецеденты темы (копия среза).
func (c *Chain) ByTopic(topic string) []Block {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Block(nil), c.topic[topic]...)
}

// Len — число блоков в цепочке.
func (c *Chain) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.blocks)
}

// Verify — проверка целостности: каждый блок ссылается на хеш предыдущего
// и собственный хеш пересчитывается.
func (c *Chain) Verify() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	prev := ""
	for _, b := range c.blocks {
		if b.PrevHash != prev {
			return false
		}
		payloadBytes, _ := json.Marshal(b.Payload)
		h := sha256.New()
		h.Write([]byte(b.PrevHash + b.Topic + b.Source))
		h.Write(payloadBytes)
		if hex.EncodeToString(h.Sum(nil)) != b.Hash {
			return false
		}
		prev = b.Hash
	}
	return true
}
