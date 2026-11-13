package scene

import (
	"math"
	"testing"
)

func TestDescriptionValidation(t *testing.T) {
	mesh := Node{ID: "card_H_02", Kind: "box", Size: Vec3{1, 1.4, .04}, Material: Material{Color: "#ffffff", Ink: "#111111", Text: "2♥"}, Transform: Identity(), PickID: "card_H_02"}
	d := New(7, "#123456", Camera{Position: Vec3{0, 0, 8}, Target: Vec3{}, FOV: 45}, mesh)
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Description){
		"version":        func(x *Description) { x.Version = 2 },
		"duplicate":      func(x *Description) { x.Nodes = append(x.Nodes, x.Nodes[0]) },
		"missing parent": func(x *Description) { x.Nodes[0].Parent = "missing" },
		"invalid mesh":   func(x *Description) { x.Nodes[0].Size[0] = 0 },
		"nonfinite":      func(x *Description) { x.Nodes[0].Transform.Position[0] = math.NaN() },
	} {
		t.Run(name, func(t *testing.T) {
			bad := d
			bad.Nodes = append([]Node(nil), d.Nodes...)
			change(&bad)
			if bad.Validate() == nil {
				t.Fatal("accepted invalid scene")
			}
		})
	}
}
