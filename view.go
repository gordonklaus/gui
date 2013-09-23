package gui

import (
	. "code.google.com/p/gordon-go/util"
	gl "github.com/chsc/gogl/gl21"
)

type KeyEvent struct {
	Key                     int
	Action                  int
	Text                    string // only present on Press and Repeat, not Release
	Shift, Ctrl, Alt, Super bool
}

type View interface {
	base() *ViewBase

	Parent() View
	Children() []View
	AddChild(View)
	RemoveChild(View)

	Show()
	Hide()

	Raise()
	raiseChild(View)
	Lower()
	lowerChild(View)

	Position() Point
	Move(Point)

	Rect() Rectangle
	SetRect(Rectangle)

	SetKeyboardFocus(View)
	GetKeyboardFocus() View
	TakeKeyboardFocus()
	TookKeyboardFocus()
	LostKeyboardFocus()
	KeyPressed(KeyEvent)
	KeyReleased(KeyEvent)

	SetMouseFocus(focus MouseHandlerView, button int)
	GetMouseFocus(button int, p Point) MouseHandlerView

	Repaint()
	Paint()

	Do(func())
}

type MouseHandlerView interface {
	View
	MouseHandler
}

type ViewBase struct {
	Self     View
	parent   View
	children []View
	hidden   bool
	rect     Rectangle
	position Point
}

func NewView(self View) *ViewBase {
	v := &ViewBase{}
	if self == nil {
		self = v
	}
	v.Self = self
	return v
}

func (v *ViewBase) base() *ViewBase { return v }

func (v ViewBase) Parent() View         { return v.parent }
func (v ViewBase) Children() []View     { return v.children }
func (v *ViewBase) AddChild(child View) {
	if child.Parent() != nil {
		child.Parent().RemoveChild(child)
	}
	v.children = append(v.children, child)
	child.base().parent = v.Self
	child.Repaint()
}
func (v *ViewBase) RemoveChild(child View) {
	SliceRemove(&v.children, child)
	child.base().parent = nil
	v.Self.Repaint()
}
func Close(v View) {
	if v.Parent() != nil {
		v.Parent().RemoveChild(v)
	}
}

func (v *ViewBase) Show() { v.hidden = false; v.Self.Repaint() }
func (v *ViewBase) Hide() { v.hidden = true; v.Self.Repaint() }

func (v *ViewBase) Raise() {
	if v.parent != nil {
		v.parent.raiseChild(v.Self)
	}
}
func (v *ViewBase) raiseChild(child View) {
	for i, view := range v.children {
		if view == child {
			v.children = append(append(v.children[:i], v.children[i+1:]...), view)
			v.Self.Repaint()
			return
		}
	}
}
func (v *ViewBase) Lower() {
	if v.parent != nil {
		v.parent.lowerChild(v.Self)
	}
}
func (v *ViewBase) lowerChild(child View) {
	for i, view := range v.children {
		if view == child {
			v.children = append(v.children[i:i+1], append(v.children[:i], v.children[i+1:]...)...)
			v.Self.Repaint()
			return
		}
	}
}

func (v ViewBase) Position() Point { return v.position }
func (v *ViewBase) Move(p Point) {
	v.position = p
	v.Self.Repaint()
}
func MoveCenter(v View, p Point) { v.Move(p.Sub(Size(v).Div(2))) }
func MoveOrigin(v View, p Point) { v.Move(p.Add(v.Rect().Min)) }

func (v ViewBase) Rect() Rectangle { return v.rect }
func Center(v View) Point          { return v.Rect().Min.Add(Size(v).Div(2)) }
func Size(v View) Point            { return v.Rect().Size() }
func Width(v View) float64         { return v.Rect().Dx() }
func Height(v View) float64        { return v.Rect().Dy() }
func (v *ViewBase) SetRect(r Rectangle) { v.rect = r; v.Repaint() }
func Pan(v View, p Point) {
	r := v.Rect()
	v.SetRect(r.Add(p.Sub(r.Min)))
}
func Resize(v View, s Point) {
	r := v.Rect()
	r.Max = r.Min.Add(s)
	v.SetRect(r)
}
func ResizeToFit(v View, margin float64) {
	if len(v.Children()) == 0 {
		v.SetRect(ZR)
		return
	}
	c1 := v.Children()[0]
	rect := MapRectToParent(c1, c1.Rect())
	for _, c := range v.Children() {
		rect = rect.Union(MapRectToParent(c, c.Rect()))
	}
	v.SetRect(rect.Inset(-margin))
}

func (v *ViewBase) SetKeyboardFocus(view View) {
	if v.parent != nil {
		v.parent.SetKeyboardFocus(view)
	}
}
func (v ViewBase) GetKeyboardFocus() View {
	if v.parent != nil {
		return v.parent.GetKeyboardFocus()
	}
	return nil
}
func (v *ViewBase) TakeKeyboardFocus() { v.Self.SetKeyboardFocus(v.Self) }
func (v *ViewBase) TookKeyboardFocus() {}
func (v *ViewBase) LostKeyboardFocus() {}
func (v *ViewBase) KeyPressed(event KeyEvent) {
	if v.parent != nil {
		v.parent.KeyPressed(event)
	}
}
func (v *ViewBase) KeyReleased(event KeyEvent) {
	if v.parent != nil {
		v.parent.KeyReleased(event)
	}
}

func (v *ViewBase) SetMouseFocus(focus MouseHandlerView, button int) {
	if v.parent != nil {
		v.parent.SetMouseFocus(focus, button)
	}
}
func (v *ViewBase) GetMouseFocus(button int, p Point) MouseHandlerView {
	if !p.In(v.Rect()) {
		return nil
	}
	children := v.Self.Children()
	for i := len(children) - 1; i >= 0; i-- {
		if c := children[i].GetMouseFocus(button, MapFromParent(children[i], p)); c != nil {
			return c
		}
	}
	f, _ := v.Self.(MouseHandlerView)
	return f
}

func (v ViewBase) Repaint() {
	if v.parent != nil {
		v.parent.Repaint()
	}
}
func (v ViewBase) paint() {
	if v.hidden {
		return
	}
	gl.PushMatrix()
	defer gl.PopMatrix()
	delta := v.Position().Sub(v.Rect().Min)
	gl.Translated(gl.Double(delta.X), gl.Double(delta.Y), 0)
	v.Self.Paint()
	for _, child := range v.Self.Children() {
		child.base().paint()
	}
}
func (v ViewBase) Paint() {}

func (v ViewBase) Do(f func()) {
	if v.parent != nil {
		v.parent.Do(f)
	}
}

func ViewAt(v View, point Point) View {
	if !point.In(v.Rect()) {
		return nil
	}
	children := v.Children()
	for i := len(children) - 1; i >= 0; i-- {
		child := children[i]
		view := ViewAt(child, MapFromParent(child, point))
		if view != nil {
			return view
		}
	}
	return v
}

func MapFromParent(v View, point Point) Point {
	return point.Sub(v.Position()).Add(v.Rect().Min)
}
func MapFrom(v View, point Point, parent View) Point {
	if v == parent || v.Parent() == nil {
		return point
	}
	return MapFromParent(v, MapFrom(v.Parent(), point, parent))
}
func MapToParent(v View, point Point) Point {
	return point.Sub(v.Rect().Min).Add(v.Position())
}
func MapTo(v View, point Point, parent View) Point {
	if v == parent || v.Parent() == nil {
		return point
	}
	return MapTo(v.Parent(), MapToParent(v, point), parent)
}

func MapRectFromParent(v View, rect Rectangle) Rectangle {
	return Rectangle{MapFromParent(v, rect.Min), MapFromParent(v, rect.Max)}
}
func MapRectFrom(v View, rect Rectangle, parent View) Rectangle {
	return Rectangle{MapFrom(v, rect.Min, parent), MapFrom(v, rect.Max, parent)}
}
func MapRectToParent(v View, rect Rectangle) Rectangle {
	return Rectangle{MapToParent(v, rect.Min), MapToParent(v, rect.Max)}
}
func MapRectTo(v View, rect Rectangle, parent View) Rectangle {
	return Rectangle{MapTo(v, rect.Min, parent), MapTo(v, rect.Max, parent)}
}
