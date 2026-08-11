// Example Go file for testing

package main

import (
	"fmt"
	"strings"
)

type Processor struct {
	name string
	data []string
}

func NewProcessor(name string) *Processor {
	return &Processor{
		name: name,
		data: make([]string, 0),
	}
}

func (p *Processor) Add(item string) {
	p.data = append(p.data, item)
}

func (p *Processor) Process() string {
	return strings.Join(p.data, ", ")
}

func main() {
	proc := NewProcessor("test")
	proc.Add("hello")
	proc.Add("world")
	fmt.Println(proc.Process())
}