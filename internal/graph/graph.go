package graph

import (
	"bufio"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unsafe"

	xmlparser "github.com/tamerh/xml-stream-parser"
)

type Node struct {
	ID   int     `xml:"id,attr,string"`
	Cost float64 `xml:"cost"`
}

func (n *Node) unmarshalXML(dec *xml.Decoder, s xml.StartElement) error {
	var err error

	id := findAttribute(s.Attr, "id")
	if id == nil {
		return errors.New("node has no id attribute")
	}

	n.ID, err = strconv.Atoi(id.Value)
	if err != nil {
		return fmt.Errorf("node has invalid id value: %w", err)
	}

	var inCost bool
	var depth int

	for {
		it, err := dec.RawToken()
		if err != nil {
			return err
		}

		switch t := it.(type) {
		case xml.StartElement:
			depth++
			inCost = t.Name.Local == "cost" && depth == 1
		case xml.CharData:
			if !inCost {
				break
			}

			n.Cost, err = strconv.ParseFloat(strings.TrimSpace(*(*string)(unsafe.Pointer(&t))), 64)
			if err != nil {
				return fmt.Errorf("node has invalid cost: %w", err)
			}

			inCost = false
		case xml.EndElement:
			depth--
			if t.Name.Local == "node" && depth == -1 {
				return nil
			}
		}
	}
}

type directed bool

func (d *directed) UnmarshalXMLAttr(a xml.Attr) error {
	*d = a.Value == "yes"

	return nil
}

type Edge struct {
	Cost     float64  `xml:"cost"`
	Directed directed `xml:"directed,attr"`
	Src      int      `xml:"source"`
	Dst      int      `xml:"target"`
}

func (e *Edge) unmarshalXML(dec *xml.Decoder, s xml.StartElement) error {
	if directed := findAttribute(s.Attr, "directed"); directed != nil {
		e.Directed = directed.Value == "yes"
	}

	var inCost, inSrc, inDst bool
	var depth int

	for {
		it, err := dec.RawToken()
		if err != nil {
			return err
		}

		switch t := it.(type) {
		case xml.StartElement:
			depth++
			if depth > 1 {
				break
			}

			n := t.Name.Local
			inCost = n == "cost"
			inSrc = n == "source"
			inDst = n == "target"
		case xml.CharData:
			s := *(*string)(unsafe.Pointer(&t))
			if inCost {
				e.Cost, err = strconv.ParseFloat(strings.TrimSpace(s), 64)
				if err != nil {
					return fmt.Errorf("edge has invalid cost: %w", err)
				}
				inCost = false
			} else if inSrc || inDst {
				v, err := strconv.Atoi(strings.TrimSpace(s))
				if err != nil {
					return fmt.Errorf("edge has invalid source or target: %w", err)
				}

				if inSrc {
					e.Src = v
					inSrc = false
				} else {
					e.Dst = v
					inDst = false
				}
			}
		case xml.EndElement:
			depth--
			if t.Name.Local == "edge" && depth == -1 {
				return nil
			}
		}
	}
}

type Graph struct {
	Nodes []Node `xml:"node"`
	Edges []Edge `xml:"edge"`
}

func FromXML(r io.Reader) (Graph, error) {
	var g Graph
	dec := xml.NewDecoder(r)

	for {
		it, err := dec.RawToken()
		if err == io.EOF {
			return g, nil
		}
		if err != nil {
			return Graph{}, err
		}

		t, ok := it.(xml.StartElement)
		if !ok {
			continue
		}

		switch t.Name.Local {
		case "node":
			var node Node
			if err := node.unmarshalXML(dec, t); err != nil {
				return Graph{}, err
			}

			g.Nodes = append(g.Nodes, node)
		case "edge":
			var edge Edge
			if err := edge.unmarshalXML(dec, t); err != nil {
				return Graph{}, err
			}

			g.Edges = append(g.Edges, edge)
		}
	}
}

func getChild(children map[string][]xmlparser.XMLElement, name string) *xmlparser.XMLElement {
	arr := children[name]
	if len(arr) == 0 {
		return nil
	}
	return &arr[0]
}

func FromXMLNoStd(r *bufio.Reader) (Graph, error) {
	p := xmlparser.NewXMLParser(r, "node", "edge")
	var g Graph
	var err error

	for e := range p.Stream() {
		switch e.Name {
		case "node":
			var node Node
			node.ID, err = strconv.Atoi(e.Attrs["id"])
			if err != nil {
				return Graph{}, fmt.Errorf("node has invalid id: %w", err)
			}

			if cost := getChild(e.Childs, "cost"); cost != nil {
				node.Cost, err = strconv.ParseFloat(cost.InnerText, 64)
				if err != nil {
					return Graph{}, fmt.Errorf("node has invalid cost: %w", err)
				}
			}

			g.Nodes = append(g.Nodes, node)
		case "edge":
			var edge Edge
			edge.Directed = e.Attrs["directed"] == "yes"

			src := getChild(e.Childs, "source")
			dst := getChild(e.Childs, "target")

			if src == nil || dst == nil {
				return Graph{}, errors.New("edge does not have source or target")
			}

			edge.Src, err = strconv.Atoi(src.InnerText)
			if err == nil {
				edge.Dst, err = strconv.Atoi(dst.InnerText)
			}

			if err != nil {
				return Graph{}, fmt.Errorf("edge has invalid source or target: %w", err)
			}

			if cost := getChild(e.Childs, "cost"); cost != nil {
				edge.Cost, err = strconv.ParseFloat(cost.InnerText, 64)
				if err != nil {
					return Graph{}, fmt.Errorf("edge has invalid cost: %w", err)
				}
			}

			g.Edges = append(g.Edges, edge)
		}
	}

	return g, nil
}

func findAttribute(attrs []xml.Attr, attr string) *xml.Attr {
	for i := range attrs {
		if attrs[i].Name.Local == attr {
			return &attrs[i]
		}
	}

	return nil
}
