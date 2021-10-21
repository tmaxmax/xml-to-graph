package graph

import "encoding/xml"

type directed bool

func (d *directed) UnmarshalXMLAttr(attr xml.Attr) error {
	*d = attr.Value == "yes"
	return nil
}

type Node struct {
	ID   int     `xml:"id,attr,string"`
	Cost float64 `xml:"cost"`
}

type Edge struct {
	Cost     float64  `xml:"cost"`
	Directed directed `xml:"directed,attr"`
	Src      int      `xml:"source"`
	Dst      int      `xml:"target"`
}

type Graph struct {
	Nodes []Node `xml:"node"`
	Edges []Edge `xml:"edge"`
}
