// Package scene defines the small, versioned 3D presentation schema.
package scene

import (
	"fmt"
	"math"
	"regexp"
)

const Version = 1

var idPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type Vec3 [3]float64

type Transform struct {
	Position Vec3 `json:"position"`
	Rotation Vec3 `json:"rotation"`
	Scale    Vec3 `json:"scale"`
}

func Identity() Transform { return Transform{Scale: Vec3{1, 1, 1}} }

type Camera struct {
	Position Vec3    `json:"position"`
	Target   Vec3    `json:"target"`
	FOV      float64 `json:"fov"`
}

type Material struct {
	Color string `json:"color"`
	Text  string `json:"text,omitempty"`
	Ink   string `json:"ink,omitempty"`
}

type Node struct {
	ID        string    `json:"id"`
	Parent    string    `json:"parent,omitempty"`
	Kind      string    `json:"kind"` // group or box
	Size      Vec3      `json:"size,omitempty"`
	Material  Material  `json:"material,omitempty"`
	Transform Transform `json:"transform"`
	PickID    string    `json:"pickId,omitempty"`
}

type Description struct {
	Version    int    `json:"version"`
	Revision   uint64 `json:"revision"`
	Background string `json:"background"`
	Camera     Camera `json:"camera"`
	Nodes      []Node `json:"nodes"`
}

func New(revision uint64, background string, camera Camera, nodes ...Node) Description {
	return Description{Version: Version, Revision: revision, Background: background, Camera: camera, Nodes: nodes}
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func validVec(v Vec3) bool  { return finite(v[0]) && finite(v[1]) && finite(v[2]) }
func validTransform(t Transform) bool {
	return validVec(t.Position) && validVec(t.Rotation) && validVec(t.Scale) &&
		t.Scale[0] > 0 && t.Scale[1] > 0 && t.Scale[2] > 0
}

func (d Description) Validate() error {
	if d.Version != Version || !colorPattern.MatchString(d.Background) || !validVec(d.Camera.Position) || !validVec(d.Camera.Target) ||
		!finite(d.Camera.FOV) || d.Camera.FOV < 20 || d.Camera.FOV > 100 || len(d.Nodes) > 128 {
		return fmt.Errorf("invalid scene header")
	}
	seen := make(map[string]bool, len(d.Nodes))
	for _, n := range d.Nodes {
		if !idPattern.MatchString(n.ID) || seen[n.ID] || !validTransform(n.Transform) {
			return fmt.Errorf("invalid or duplicate scene node %q", n.ID)
		}
		if n.Parent != "" && (!seen[n.Parent] || n.Parent == n.ID) {
			return fmt.Errorf("node %q must follow its parent", n.ID)
		}
		if n.Kind != "group" && n.Kind != "box" {
			return fmt.Errorf("unsupported node kind %q", n.Kind)
		}
		if n.Kind == "box" {
			if !validVec(n.Size) || n.Size[0] <= 0 || n.Size[1] <= 0 || n.Size[2] <= 0 ||
				!colorPattern.MatchString(n.Material.Color) || (n.Material.Ink != "" && !colorPattern.MatchString(n.Material.Ink)) || len(n.Material.Text) > 32 {
				return fmt.Errorf("invalid mesh %q", n.ID)
			}
		}
		if n.PickID != "" && !idPattern.MatchString(n.PickID) {
			return fmt.Errorf("invalid pick ID on %q", n.ID)
		}
		seen[n.ID] = true
	}
	return nil
}
