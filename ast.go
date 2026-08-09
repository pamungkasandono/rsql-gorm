package rsql

type Node interface {
	node()
}

type ComparisonNode struct {
	Selector  string
	Operator  string
	Arguments any
}

func (ComparisonNode) node() {}

type AndNode struct {
	Children []Node
}

func (AndNode) node() {}

type OrNode struct {
	Children []Node
}

func (OrNode) node() {}
