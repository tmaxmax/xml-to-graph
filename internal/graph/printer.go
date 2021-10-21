package graph

import (
	"bufio"
	"bytes"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"
)

type writer interface {
	io.Writer
	io.ByteWriter
	io.StringWriter
}

func grow(w writer, n int) {
	g, ok := w.(interface{ Grow(int) })
	if ok {
		g.Grow(n)
	}
}

func flush(w writer) error {
	f, ok := w.(interface{ Flush() error })
	if ok {
		return f.Flush()
	}
	return nil
}

type operation interface {
	apply(writer, *Graph) (int, error)
}

type operationFunc func(writer, *Graph) (int, error)

func (o operationFunc) apply(w writer, g *Graph) (int, error) {
	return o(w, g)
}

// ParsePrinterError is the type of error returned by ParseError
// when the format string for the printer is invalid.
type ParsePrinterError struct {
	// Any underlying error that may have occurred when parsing.
	Reason error
	// The part of the format string where this error occured.
	Format string
	// Additional details about the error.
	Explanation string
}

func (p *ParsePrinterError) Error() string {
	r := "invalid format string"
	if p.Explanation != "" {
		r += ": " + p.Explanation
	}
	if p.Format != "" {
		r += " (in \"" + p.Format + "\")"
	}
	if p.Reason != nil {
		r += ": " + p.Reason.Error()
	}
	return r
}

func (p *ParsePrinterError) Unwrap() error {
	return p.Reason
}

// A Printer writes a representation of a graph to an io.Writer.
// Use ParsePrinter or MustParsePrinter to create a Printer.
//
// By default writes are buffered using a bufio.Writer, unless the
// provided writer is already a bufio.Writer, a strings.Builder, or
// a bytes.Buffer. A Printer is safe for concurrent use. It uses a
// sync.Pool of bufio.Writer so resources can be reused when batch
// printing multiple graphs.
type Printer struct {
	ops []operation
	bwp sync.Pool
}

func (p *Printer) getWriter(w io.Writer) writer {
	switch v := w.(type) {
	case *bytes.Buffer:
		return v
	case *strings.Builder:
		return v
	case *bufio.Writer:
		return v
	default:
		bw := p.bwp.Get().(*bufio.Writer)
		defer p.bwp.Put(bw)
		bw.Reset(w)
		return bw
	}
}

// Print writes the given graph to the given writer.
// It returns the number of bytes written and any error
// that the underlying writer may have returned. If the writer
// returns no errors, this error can be ignored.
func (p *Printer) Print(w io.Writer, g *Graph) (int, error) {
	wr, n := p.getWriter(w), 0

	for _, op := range p.ops {
		m, err := op.apply(wr, g)
		n += m
		if err != nil {
			return n, err
		}
	}

	return n, flush(wr)
}

// ParsePrinter creates a printer from the given format string.
//
// The format strings are C-like (prefixed with %) and have the following verbs:
//  - %: print a literal "%"
//  - n: print the number of vertices a graph has
//  - m: print the number of edges a graph has
//  - {cost function}w: print the cost/weight of each vertex
//  - {cost function}N: print each vertex, optionally together with its cost
//  - {cost function}M: print each edge, optionally together with its cost
//
// A cost function returns a new cost based on the actual one. It is useful for
// adapting the output to your needs: for example, round the cost to the nearest
// integer. A cost function is defined as following:
//    {ratio}{rounding mode}
// The ratio is a factor with which the cost is multiplied consisting of
// ASCII digits and optionally the dot (".") character. For example, the
// following are valid ratios:
//  - .5: ratio of 0.5
//  - 10: ratio of 10
//  - 0.6: ratio of 0.6
// The rounding mode is optional. It can have the following values:
//  - X (default): no rounding
//  - F: flooring function
//  - C: ceiling function
//  - R: rounding to nearest integer function
// Where a cost function is required for a verb, but none is provided,
// the identity cost function is used (ratio 1, no rounding).
func ParsePrinter(format string) (*Printer, error) {
	if format == "" {
		return nil, &ParsePrinterError{
			Explanation: "required to be non-empty",
		}
	}

	p := &Printer{
		bwp: sync.Pool{
			New: func() interface{} {
				return bufio.NewWriter(nil)
			},
		},
	}

	for {
		i := strings.IndexByte(format, '%')
		if i == -1 {
			i = len(format)
		}

		text := format[:i]
		if text != "" {
			p.ops = append(p.ops, textOperation(text))
		}

		if i == len(format) {
			break
		}

		op, advance, err := parseArg(format[i+1:])
		if err != nil {
			return nil, err
		}

		format = format[i+advance+1:]
		p.ops = append(p.ops, op)
	}

	return p, nil
}

// MustParsePrinter is the same as ParsePrinter, but panics on non-nil error.
// Use this if you are sure the format string is valid. See the documentation for
// ParsePrinter to see how a format string is built.
func MustParsePrinter(format string) *Printer {
	p, err := ParsePrinter(format)
	if err != nil {
		panic(err)
	}

	return p
}

const (
	verbLiteralPercent  = '%'
	verbVerticesCount   = 'n'
	verbEdgesCount      = 'm'
	verbAdjacencyMatrix = 'a'
	verbCosts           = 'w'
	verbVertices        = 'N'
	verbEdges           = 'M'
)

func parseArg(text string) (operation, int, error) {
	if text == "" {
		return nil, 0, &ParsePrinterError{
			Explanation: "unexpected end",
		}
	}

	switch text[0] {
	case verbLiteralPercent:
		return textOperation("%"), 1, nil
	case verbVerticesCount:
		return verticesCountOperation, 1, nil
	case verbEdgesCount:
		return edgesCountOperation, 1, nil
	case verbAdjacencyMatrix:
		return adjacencyMatrixOperation, 1, nil
	default:
		costFn, advance, err := parseCostFunction(text)
		if err != nil {
			return nil, 0, err
		}

		if advance == len(text) {
			return nil, 0, &ParsePrinterError{
				Format:      text,
				Explanation: "missing verb",
			}
		}

		switch c := text[advance]; c {
		case verbCosts:
			if costFn == nil {
				return costsOperation(costFunction{ratio: 1, round: noopRound}), advance + 1, nil
			}
			return costsOperation(*costFn), advance + 1, nil
		case verbVertices:
			return &verticesOperation{cost: costFn}, advance + 1, nil
		case verbEdges:
			return &edgesOperation{cost: costFn}, advance + 1, nil
		default:
			return nil, 0, &ParsePrinterError{
				Format:      text,
				Explanation: "invalid verb \"" + string(c) + "\"",
			}
		}
	}
}

type costFunction struct {
	ratio float64
	round func(float64) float64
}

func (c costFunction) Cost(v float64) float64 {
	return c.round(v * c.ratio)
}

const (
	roundingModeNone  = 'X'
	roundingModeFloor = 'F'
	roundingModeRound = 'R'
	roundingModeCeil  = 'C'
)

func noopRound(v float64) float64 { return v }

func parseCostFunction(s string) (*costFunction, int, error) {
	i := strings.IndexFunc(s, func(r rune) bool {
		return (r <= '0' || r >= '9') && r != '.'
	})
	advance := i + 1
	if i == -1 {
		i = len(s)
		advance = i
	}

	fn := costFunction{ratio: 1, round: noopRound}
	if ratioStr := s[:i]; ratioStr != "" {
		var err error
		fn.ratio, err = strconv.ParseFloat(ratioStr, 64)
		if err != nil {
			return nil, 0, &ParsePrinterError{
				Format:      s,
				Explanation: "invalid cost ratio",
				Reason:      err,
			}
		}
	}

	if i != len(s) {
		switch s[i] {
		case roundingModeNone:
		case roundingModeFloor:
			fn.round = math.Floor
		case roundingModeRound:
			fn.round = math.Round
		case roundingModeCeil:
			fn.round = math.Ceil
		default:
			advance--
			if advance == 0 {
				return nil, 0, nil
			}
		}
	}

	return &fn, advance, nil
}

type textOperation string

func (t textOperation) apply(w writer, _ *Graph) (int, error) {
	return w.WriteString(string(t))
}

var (
	verticesCountOperation operationFunc = func(w writer, g *Graph) (int, error) {
		return w.Write(strconv.AppendInt(nil, int64(len(g.Nodes)), 10))
	}
	edgesCountOperation operationFunc = func(w writer, g *Graph) (int, error) {
		return w.Write(strconv.AppendInt(nil, int64(len(g.Edges)), 10))
	}
	adjacencyMatrixOperation operationFunc = func(w writer, g *Graph) (int, error) {
		grow(w, len(g.Nodes)*len(g.Nodes))
		var n int
		var err error

		for ia, a := range g.Nodes {
			if ia > 0 {
				if err = w.WriteByte('\n'); err != nil {
					return n, err
				}
				n++
			}

			for ib, b := range g.Nodes {
				if ib > 0 {
					if err = w.WriteByte(' '); err != nil {
						return n, err
					}
					n++
				}

				if a.ID == b.ID {
					if err = w.WriteByte('0'); err != nil {
						return n, err
					}
					n++
					continue
				}

				var areAdjacent bool
				for _, e := range g.Edges {
					if (e.Src == a.ID && e.Dst == b.ID) || (!e.Directed && e.Src == b.ID && e.Dst == a.ID) {
						areAdjacent = true
						break
					}
				}

				if areAdjacent {
					err = w.WriteByte('1')
				} else {
					err = w.WriteByte('0')
				}

				if err != nil {
					return n, err
				}
				n++
			}
		}

		return n, nil
	}
)

type verticesOperation struct {
	prefixCost bool
	cost       *costFunction
}

func (v *verticesOperation) apply(w writer, g *Graph) (int, error) {
	b := []byte{}
	var n, m int
	var err error

	writeCost := func(nd *Node) error {
		if v.cost == nil {
			return nil
		}
		if !v.prefixCost {
			if err = w.WriteByte(' '); err != nil {
				return err
			}
			n++
		}
		m, err = w.Write(strconv.AppendFloat(b[:0], v.cost.Cost(nd.Cost), 'f', -1, 64))
		n += m
		return err
	}
	writeVertex := func(nd *Node) error {
		if v.prefixCost && v.cost != nil {
			if err = w.WriteByte(' '); err != nil {
				return err
			}
			n++
		}
		m, err = w.Write(strconv.AppendInt(b[:0], int64(nd.ID), 10))
		n += m
		return err
	}

	for i, node := range g.Nodes {
		if i > 0 {
			if err = w.WriteByte('\n'); err != nil {
				return n, err
			}
			n++
		}

		if v.prefixCost {
			err = writeCost(&node)
			if err == nil {
				err = writeVertex(&node)
			}
		} else {
			err = writeVertex(&node)
			if err == nil {
				err = writeCost(&node)
			}
		}

		if err != nil {
			break
		}
	}

	return n, err
}

type edgesOperation struct {
	prefixCost bool
	cost       *costFunction
}

func (v *edgesOperation) apply(w writer, g *Graph) (int, error) {
	b := []byte{}
	var n, m int
	var err error

	writeCost := func(e *Edge) error {
		if v.cost == nil {
			return nil
		}
		if !v.prefixCost {
			if err = w.WriteByte(' '); err != nil {
				return err
			}
			n++
		}
		m, err = w.Write(strconv.AppendFloat(b[:0], v.cost.Cost(e.Cost), 'f', -1, 64))
		n += m
		return err
	}
	writeEdge := func(e *Edge) error {
		if v.prefixCost && v.cost != nil {
			if err = w.WriteByte(' '); err != nil {
				return err
			}
			n++
		}
		b = strconv.AppendInt(b[:0], int64(e.Src), 10)
		b = append(b, ' ')
		m, err = w.Write(strconv.AppendInt(b, int64(e.Dst), 10))
		n += m
		return err
	}

	for i, e := range g.Edges {
		if i > 0 {
			if err = w.WriteByte('\n'); err != nil {
				return n, err
			}
			n++
		}

		if v.prefixCost {
			err = writeCost(&e)
			if err == nil {
				err = writeEdge(&e)
			}
		} else {
			err = writeEdge(&e)
			if err == nil {
				err = writeCost(&e)
			}
		}

		if err != nil {
			break
		}
	}

	return n, err
}

type costsOperation costFunction

func (c costsOperation) apply(w writer, g *Graph) (int, error) {
	b := []byte{}
	fn := costFunction(c)
	var n, m int
	var err error

	for i, node := range g.Nodes {
		if i > 0 {
			if err = w.WriteByte(' '); err != nil {
				return n, err
			}
			n++
		}

		m, err = w.Write(strconv.AppendFloat(b[:0], fn.Cost(node.Cost), 'f', -1, 64))
		n += m
		if err != nil {
			break
		}
	}

	return n, err
}
