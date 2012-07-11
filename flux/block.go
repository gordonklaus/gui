package main

import (
	."fmt"
	."strings"
	."math"
	"time"
	."github.com/jteeuwen/glfw"
	."code.google.com/p/gordon-go/gui"
	."code.google.com/p/gordon-go/util"
)

type Block struct {
	*ViewBase
	AggregateMouseHandler
	node Node
	nodes map[Node]bool
	connections map[*Connection]bool
	focused, editing bool
	editingNode Node
	points []Point
}

func NewBlock(node Node) *Block {
	b := &Block{}
	b.ViewBase = NewView(b)
	b.AggregateMouseHandler = AggregateMouseHandler{NewClickKeyboardFocuser(b), NewViewPanner(b)}
	b.node = node
	b.nodes = map[Node]bool{}
	b.connections = map[*Connection]bool{}
	b.Pan(Pt(-400, -300))
	go b.reform()
	return b
}

func (b *Block) Outer() *Block {
	if b.node == nil { return nil }
	return b.node.Block()
}
func (b *Block) Outermost() *Block {
	if outer := b.Outer(); outer != nil { return outer.Outermost() }
	return b
}

func (b *Block) AddNode(node Node) {
	b.AddChild(node)
	b.nodes[node] = true
}

func (b *Block) RemoveNode(node Node) {
	b.RemoveChild(node)
	delete(b.nodes, node)
}

func (b *Block) NewConnection(pt Point) *Connection {
	conn := NewConnection(b, pt)
	b.AddConnection(conn)
	return conn
}

func (b *Block) AddConnection(conn *Connection) {
	if conn.block != nil {
		delete(conn.block.connections, conn)
	}
	conn.block = b
	b.AddChild(conn)
	conn.Lower()
	b.connections[conn] = true
}

func (b *Block) DeleteConnection(connection *Connection) {
	connection.Disconnect()
	delete(b.connections, connection)
	b.RemoveChild(connection)
}

func (b Block) AllNodes() (nodes []Node) {
	for n := range b.nodes {
		nodes = append(nodes, n)
		switch n := n.(type) {
		case *IfNode:
			nodes = append(nodes, append(n.falseBlock.AllNodes(), n.trueBlock.AllNodes()...)...)
		}
	}
	return nodes
}

func (b Block) AllConnections() (conns []*Connection) {
	for c := range b.connections {
		conns = append(conns, c)
	}
	for n := range b.nodes {
		switch n := n.(type) {
		case *IfNode:
			conns = append(conns, append(n.falseBlock.AllConnections(), n.trueBlock.AllConnections()...)...)
		}
	}
	return conns
}

func (b Block) InputConnections() (connections []*Connection) {
	for node := range b.nodes {
		for _, conn := range node.InputConnections() {
			if !b.connections[conn] {
				connections = append(connections, conn)
			}
		}
	}
	return
}

func (b Block) OutputConnections() (connections []*Connection) {
	for node := range b.nodes {
		for _, conn := range node.OutputConnections() {
			if !b.connections[conn] {
				connections = append(connections, conn)
			}
		}
	}
	return
}

func (b *Block) nodeOrder() (order []Node, ok bool) {
	visited := Set{}
	var insertInOrder func(node Node, visitedThisCall Set) bool
	insertInOrder = func(node Node, visitedThisCall Set) bool {
		if visitedThisCall[node] { return false }
		visitedThisCall[node] = true
		
		if !visited[node] {
			visited[node] = true
conns:		for _, conn := range node.InputConnections() {
				if b.connections[conn] {
					srcNode := conn.src.node
					for !b.nodes[srcNode] {
						srcNode = srcNode.Block().node
						if srcNode == nil { continue conns }
					}
					if !insertInOrder(srcNode, visitedThisCall.Copy()) { return false }
				}
			}
			order = append(order, node)
		}
		return true
	}
	
	endNodes := []Node{}
nx:	for node := range b.nodes {
		for _, conn := range node.OutputConnections() {
			if conn.block == b { continue nx }
		}
		endNodes = append(endNodes, node)
	}
	if len(endNodes) == 0 && len(b.nodes) > 0 { return }
	
	for _, node := range endNodes {
		if !insertInOrder(node, Set{}) { return }
	}
	ok = true
	return
}

func (b *Block) Code(indent int, vars map[*Input]string) (s string) {
	vars, varsCopy := map[*Input]string{}, vars
	for k, v := range varsCopy { vars[k] = v }
	
	order, ok := b.nodeOrder()
	if !ok {
		Println("cyclic!")
		return
	}
cx:	for conn := range b.connections {
		if _, ok := vars[conn.dst]; ok { continue }
		for block := conn.src.node.Block().Outer(); block != b; block = block.Outer() {
			if block == nil { continue cx }
		}
		name := newVarName()
		s += Sprintf("%vvar %v %v\n", tabs(indent), name, conn.dst.info.typeName)
		vars[conn.dst] = name
	}
	for _, node := range order {
		switch node := node.(type) {
		default:
			inputs := []string{}
			for _, input := range node.Inputs() {
				name := ""
				if len(input.connections) > 0 {
					name = vars[input.connections[0].dst]
				} else {
					// INSTEAD:  name = "*new(typeName)"  or zero literal
					name = newVarName()
					s += Sprintf("%vvar %v %v\n", tabs(indent), name, input.info.typeName)
				}
				inputs = append(inputs, name)
			}
			outputs := []string{}
			anyOutputConnections := false
			assignExisting := map[string]string{}
			for _, output := range node.Outputs() {
				name := "_"
				if len(output.connections) > 0 {
					anyOutputConnections = true
					name = newVarName()
					for _, conn := range output.connections {
						if existingName, ok := vars[conn.dst]; ok {
							assignExisting[existingName] = name
						} else {
							vars[conn.dst] = name
						}
					}
				}
				outputs = append(outputs, name)
			}
			assignment := ""
			if anyOutputConnections {
				assignment = Join(outputs, ", ") + " := "
			}
			s += Sprintf("%v%v%v\n", tabs(indent), assignment, node.Code(indent, vars, Join(inputs, ", ")))
			if len(assignExisting) > 0 {
				var existingNames, sourceNames []string
				for v1, v2 := range assignExisting {
					existingNames = append(existingNames, v1)
					sourceNames = append(sourceNames, v2)
				}
				s += Sprintf("%v%v = %v\n", tabs(indent), Join(existingNames, ", "), Join(sourceNames, ", "))
			}
		case *IfNode:
			s += node.Code(indent, vars, "")
		}
	}
	return
}

func (b *Block) StartEditing() {
	b.TakeKeyboardFocus()
	b.editing = true
}

func (b *Block) StopEditing() {
	b.editing = false
	b.editingNode = nil
}

func (b *Block) reform() {
	for {
		v := map[Node]Point{}
		center := ZP
		for n := range b.nodes {
			v[n] = ZP
			center = center.Add(n.Position())
		}
		center = center.Div(float64(len(b.nodes)))
		for n1 := range b.nodes {
			for n2 := range b.nodes {
				if n2 == n1 { continue }
				dir := n1.MapToParent(n1.Center()).Sub(n2.MapToParent(n2.Center()))
				d := Sqrt(dir.X * dir.X + dir.Y * dir.Y)
				if d > 128 { continue }
				v[n1] = v[n1].Add(dir.Mul(2 * (128 - d) / (1 + d)))
			}
		}
		for conn := range b.connections {
			src, dst := conn.src, conn.dst
			if src == nil || dst == nil { continue }
			d := dst.MapTo(dst.Position(), b).Sub(src.MapTo(src.Position(), b).Add(Pt(64, 0)))
			d.X *= 2
			d.Y /= 2
			
			srcNode := src.node; for !b.nodes[srcNode] { srcNode = srcNode.Block().node }
			dstNode := dst.node; for !b.nodes[dstNode] { dstNode = dstNode.Block().node }
			v[srcNode] = v[srcNode].Add(d)
			v[dstNode] = v[dstNode].Sub(d)
		}
		for n, v := range v {
			v = v.Add(center.Sub(n.Position()).Div(4))
			n.Move(n.Position().Add(v.Mul(2 * .033)))
		}
		
		pts := []Point{}
		for n := range b.nodes {
			r := n.MapRectToParent(n.Rect())
			pts = append(pts, r.Min, r.Max, Pt(r.Min.X, r.Max.Y), Pt(r.Max.X, r.Min.Y))
		}
		if len(pts) == 0 { pts = append(pts, ZP, Pt(0, 16), Pt(8, 8)) }
		iLowerLeft, lowerLeft := 0, pts[0]
		for i, p := range pts {
			if p.Y < lowerLeft.Y || p.Y == lowerLeft.Y && p.X < lowerLeft.X {
				iLowerLeft, lowerLeft = i, p
			}
		}
		pts[0], pts[iLowerLeft] = pts[iLowerLeft], pts[0]
		Sort(pts[1:], func(p1, p2 Point) bool {
			x := p1.Sub(lowerLeft).Cross(p2.Sub(lowerLeft))
			if x > 0 { return true }
			if x == 0 { return p1.X < p2.X }
			return false
		})
		N := len(pts)
		pts = append([]Point{pts[N-1]}, pts...)
		m := 1
		for i := 2; i <= N; i++ {
			for pts[m].Sub(pts[m - 1]).Cross(pts[i].Sub(pts[m - 1])) <= 0 {
				if m > 1 { m-- } else if i == N { break } else { i++ }
			}
			m++
			pts[m], pts[i] = pts[i], pts[m]
		}
		pts = pts[:m]
		center = ZP
		for _, p := range pts { center = center.Add(p) }
		center = center.Div(float64(len(pts)))
		for i, p := range pts {
			dir := p.Sub(center)
			d := dir.Len()
			pts[i] = p.Add(dir.Mul(32 / d))
		}
		b.points = pts
		
		rect := ZR.Add(pts[0])
		for _, p := range pts { rect = rect.Union(ZR.Add(p)) }
		// if b.editingNode != nil && !b.nodes[b.editingNode] {
		// 	p := b.editingNode.MapTo(b.editingNode.Center(), outer)
		// 	pts = append(pts, p)
		// 	rect = rect.Union(ZR.Add(p))
		// }
		// if b.editing && b.editingNode == nil && len(b.nodes) == 0 {
		// 	pts = append(pts, p.Add(Pt(-4, 32)), p.Add(Pt(4, 32)))
		// }
	
		if b.node == nil { b.Move(b.MapToParent(rect.Min)) }
		b.Pan(rect.Min)
		b.Resize(rect.Dx(), rect.Dy())
		if n, ok := b.node.(interface{positionBlocks()}); ok { n.positionBlocks() }
		
		time.Sleep(33 * time.Millisecond)
	}
}

func (b *Block) GetNearestView(views []View, point Point, directionKey int) (nearest View) {
	dir := map[int]Point{KeyLeft:{-1, 0}, KeyRight:{1, 0}, KeyUp:{0, 1}, KeyDown:{0, -1}}[directionKey]
	bestScore := 0.0
	for _, view := range views {
		d := view.MapTo(view.Center(), b).Sub(point)
		score := (dir.X * d.X + dir.Y * d.Y) / (d.X * d.X + d.Y * d.Y);
		if (score > bestScore) {
			bestScore = score
			nearest = view
		}
	}
	return
}

func (b *Block) FocusNearestView(v View, directionKey int) {
	views := []View{}
	for _, node := range b.AllNodes() {
		views = append(views, node)
		for _, p := range node.Inputs() { views = append(views, p) }
		for _, p := range node.Outputs() { views = append(views, p) }
	}
	for _, connection := range b.AllConnections() {
		views = append(views, connection)
	}
	nearest := b.GetNearestView(views, v.MapTo(v.Center(), b), directionKey)
	if nearest != nil { nearest.TakeKeyboardFocus() }
}

func (b *Block) TookKeyboardFocus() { b.focused = true; b.Repaint() }
func (b *Block) LostKeyboardFocus() { b.focused = false; b.StopEditing(); b.Repaint() }

func (b *Block) KeyPressed(event KeyEvent) {
	switch event.Key {
	case KeyLeft, KeyRight, KeyUp, KeyDown:
		outermost := b.Outermost()
		if b.editing {
			var v View = b.editingNode
			if v == nil { v = b }
			views := []View{}; for _, n := range outermost.AllNodes() { views = append(views, n) }
			if n := outermost.GetNearestView(views, v.MapTo(v.Center(), outermost), event.Key); n != nil { b.editingNode = n.(Node) }
		} else {
			outermost.FocusNearestView(b, event.Key)
		}
	case KeySpace:
		if b.editingNode != nil {
			if b.nodes[b.editingNode] {
				b.RemoveChild(b.editingNode)
				delete(b.nodes, b.editingNode)
				b.AddNode(b.editingNode)
			} else {
				b.RemoveNode(b.editingNode)
				b.nodes[b.editingNode] = true
				b.AddChild(b.editingNode)
			}
		}
	case KeyEnter:
		if b.editing {
			if b.editingNode != nil && !b.nodes[b.editingNode] {
				b.nodes[b.editingNode] = true
			}
			b.StopEditing()
		} else {
			b.StartEditing()
		}
	case KeyEsc:
		if b.editing {
			if b.editingNode != nil {
				b.editingNode = nil
			} else {
				b.StopEditing()
			}
		} else if outer := b.Outer(); outer != nil {
			outer.TakeKeyboardFocus()
		}
	default:
		switch event.Text {
		default:
			creator := NewNodeCreator(b)
			creator.Move(b.Center())
			creator.created.Connect(func(n ...interface{}) {
				node := n[0].(Node)
				b.AddNode(node)
				node.MoveCenter(b.Center())
				node.TakeKeyboardFocus()
			})
			creator.canceled.Connect(func(...interface{}) { b.TakeKeyboardFocus() })
			creator.text.KeyPressed(event)
		case "\"":
			node := NewStringConstantNode(b)
			b.AddNode(node)
			node.MoveCenter(b.Center())
			node.text.TakeKeyboardFocus()
		case ",":
			node := NewIfNode(b)
			b.AddNode(node)
			node.MoveCenter(b.Center())
			node.TakeKeyboardFocus()
		case "":
			b.ViewBase.KeyPressed(event)
		}
	}
}

// func (b *Block) MousePressed(button int, pt Point) {
// 	b.TakeKeyboardFocus()
// 	// conn := p.node.Block().NewConnection(p.MapTo(pt, p.node.Block()))
// 	// p.spec.ConnectTo(conn)
// 	// p.spec.PassMouseFocusToFreeConnectionHandle(conn, button)
// 	// conn.StartEditing()
// }
// func (b Block) MouseDragged(button int, pt Point) {}
// func (b Block) MouseReleased(button int, pt Point) {}
// 
func (b Block) Paint() {
	if b.editing {
		SetColor(Color{.7, .4, 0, 1})
	} else if b.focused {
		SetColor(Color{.3, .3, .7, 1})
	} else {
		SetColor(Color{.5, .5, .5, 1})
	}
	n := len(b.points)
	for i := range b.points {
		p1, p2, p3 := b.points[i], b.points[(i + 1) % n], b.points[(i + 2) % n]
		p1, p3 = p1.Add(p2).Div(2), p2.Add(p3).Div(2)
		DrawQuadratic([3]Point{p1, p2, p3}, int(p3.Sub(p2).Len() + p2.Sub(p1).Len()) / 8)
	}
}