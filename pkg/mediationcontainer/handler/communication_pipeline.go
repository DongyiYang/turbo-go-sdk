package handler

import (
	"errors"

	goproto "github.com/golang/protobuf/proto"
)

type RawMessageHandler interface {
	HandleRawMessage(rawMessage []byte) (goproto.Message, error)
}

type Pipeline struct {
	queue []RawMessageHandler
}

func NewPipeline() *Pipeline {
	return &Pipeline{
		queue: []RawMessageHandler{},
	}
}

func (p *Pipeline) Push(n RawMessageHandler) {
	p.queue = append(p.queue, n)
}

func (p *Pipeline) Pop() (RawMessageHandler, error) {
	if len(p.queue) < 1 {
		return nil, errors.New("Empty pipeline")
	}
	n := p.queue[0]
	p.queue = p.queue[1:]
	return n, nil
}

func (p *Pipeline) Peek() (RawMessageHandler, error) {
	if len(p.queue) < 1 {
		return nil, errors.New("Empty pipeline")
	}
	return p.queue[0], nil
}

func (p *Pipeline) Len() int {
	return len(p.queue)
}
