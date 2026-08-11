package rsql

// Node is a node in the RSQL AST produced by Parse.
type Node interface {
	node()
}

// ComparisonNode is a single comparison of Selector against Arguments using
// Operator. Arguments is a string for the binary operators and a []string
// for =in= and =out=.
type ComparisonNode struct {
	Selector  string
	Operator  string
	Arguments any
}

func (ComparisonNode) node() {}

// AndNode combines its children with logical AND (RSQL ;).
type AndNode struct {
	Children []Node
}

func (AndNode) node() {}

// OrNode combines its children with logical OR (RSQL ,).
type OrNode struct {
	Children []Node
}

func (OrNode) node() {}
