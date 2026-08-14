// Package snowflake provides a very simple Twitter snowflake generator and parser.
package snowflake

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"
	_ "unsafe"
)

var (
	Epoch    int64 = 1288834974657
	NodeBits uint8 = 10
	StepBits uint8 = 12

	nodeMax   int64 = -1 ^ (-1 << NodeBits)
	nodeMask  int64 = nodeMax << StepBits
	stepMask  int64 = -1 ^ (-1 << StepBits)
	timeShift uint8 = NodeBits + StepBits
	nodeShift uint8 = StepBits
)

const encodeBase32Map = "ybndrfg8ejkmcpqxot1uwisza345h769"

var decodeBase32Map [256]byte

const encodeBase58Map = "123456789abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ"

var decodeBase58Map [256]byte

type JSONSyntaxError struct{ original []byte }

func (j JSONSyntaxError) Error() string {
	return fmt.Sprintf("invalid snowflake ID %q", string(j.original))
}

func init() {
	for i := range encodeBase58Map {
		decodeBase58Map[i] = 0xFF
	}
	for i := range encodeBase58Map {
		decodeBase58Map[encodeBase58Map[i]] = byte(i)
	}

	for i := range encodeBase32Map {
		decodeBase32Map[i] = 0xFF
	}
	for i := range encodeBase32Map {
		decodeBase32Map[encodeBase32Map[i]] = byte(i)
	}
}

var (
	ErrInvalidBase58 = errors.New("invalid base58")
	ErrInvalidBase32 = errors.New("invalid base32")
)

type Node struct {
	time int64
	node int64
	step int64
	mu   sync.Mutex
}

type ID int64

// NewNode 移除重复全局变量赋值，规范简洁
func NewNode(node int64) (*Node, error) {
	if node < 0 || node > nodeMax {
		return nil, errors.New("Node number must be between 0 and " + strconv.FormatInt(nodeMax, 10))
	}
	return &Node{time: 0, node: node, step: 0}, nil
}

//go:linkname now time.now
func now() (sec int64, nsec int32, mono int64)

// GetNow 高性能时间获取（保留你想要的写法）
func (n *Node) GetNow() int64 {
	s, m, _ := now()
	return (s*1e9 + int64(m)) / 1e6
}

const maxClockDrift = 2000

// Generate 核心逻辑完全保留，无任何修改
func (n *Node) Generate() ID {
	n.mu.Lock()
	defer n.mu.Unlock()

	now := n.GetNow()

	// 循环处理时钟回拨
	for n.time > now {
		drift := n.time - now
		if drift > maxClockDrift {
			panic(fmt.Sprintf("clock stepped back too far: %d ms", drift))
		}
		time.Sleep(time.Duration(drift+1) * time.Millisecond)
		now = n.GetNow()
	}

	if n.time == now {
		n.step = (n.step + 1) & stepMask
		if n.step == 0 {
			for now <= n.time {
				time.Sleep(1 * time.Millisecond)
				now = n.GetNow()
			}
		}
	} else {
		n.step = 0
	}

	n.time = now
	return ID((now-Epoch)<<timeShift | (n.node << nodeShift) | n.step)
}

// 以下所有方法完全不变，100%保留你的代码
func (f ID) Int64() int64      { return int64(f) }
func (f ID) String() string    { return strconv.FormatInt(int64(f), 10) }
func (f ID) Base2() string     { return strconv.FormatInt(int64(f), 2) }
func (f ID) Base36() string    { return strconv.FormatInt(int64(f), 36) }
func (f ID) Time() int64       { return (int64(f) >> timeShift) + Epoch }
func (f ID) Node() int64       { return int64(f) & nodeMask >> nodeShift }
func (f ID) Step() int64       { return int64(f) & stepMask }
func (f ID) IntBytes() [8]byte { var b [8]byte; binary.BigEndian.PutUint64(b[:], uint64(f)); return b }

func (f ID) Base32() string {
	if f < 32 {
		return string(encodeBase32Map[f])
	}
	b := make([]byte, 0, 12)
	for f >= 32 {
		b = append(b, encodeBase32Map[f%32])
		f /= 32
	}
	b = append(b, encodeBase32Map[f])
	for x, y := 0, len(b)-1; x < y; x, y = x+1, y-1 {
		b[x], b[y] = b[y], b[x]
	}
	return string(b)
}

func ParseBase32(b []byte) (ID, error) {
	var id int64
	for i := range b {
		if decodeBase32Map[b[i]] == 0xFF {
			return -1, ErrInvalidBase32
		}
		id = id*32 + int64(decodeBase32Map[b[i]])
	}
	return ID(id), nil
}

func (f ID) Base58() string {
	if f < 58 {
		return string(encodeBase58Map[f])
	}
	b := make([]byte, 0, 11)
	for f >= 58 {
		b = append(b, encodeBase58Map[f%58])
		f /= 58
	}
	b = append(b, encodeBase58Map[f])
	for x, y := 0, len(b)-1; x < y; x, y = x+1, y-1 {
		b[x], b[y] = b[y], b[x]
	}
	return string(b)
}

func ParseBase58(b []byte) (ID, error) {
	var id int64
	for i := range b {
		if decodeBase58Map[b[i]] == 0xFF {
			return -1, ErrInvalidBase58
		}
		id = id*58 + int64(decodeBase58Map[b[i]])
	}
	return ID(id), nil
}

func (f ID) MarshalJSON() ([]byte, error) {
	buff := make([]byte, 0, 22)
	buff = append(buff, '"')
	buff = strconv.AppendInt(buff, int64(f), 10)
	buff = append(buff, '"')
	return buff, nil
}

func (f *ID) UnmarshalJSON(b []byte) error {
	if len(b) < 3 || b[0] != '"' || b[len(b)-1] != '"' {
		return JSONSyntaxError{b}
	}
	i, err := strconv.ParseInt(string(b[1:len(b)-1]), 10, 64)
	if err != nil {
		return err
	}
	*f = ID(i)
	return nil
}
