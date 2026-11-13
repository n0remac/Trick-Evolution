// Package animation describes bounded local presentation motions.
package animation

import (
	"fmt"
	"math"
	"regexp"

	"github.com/n0remac/trick-evolution/scene"
)

const Version = 1

var idPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

type Step struct {
	To         scene.Transform `json:"to"`
	DurationMS int             `json:"durationMs"`
	Easing     string          `json:"easing"`
}

type Description struct {
	Version int    `json:"version"`
	Target  string `json:"target"`
	Steps   []Step `json:"steps"`
}

func New(target string, steps ...Step) Description {
	return Description{Version: Version, Target: target, Steps: steps}
}

func (d Description) Validate() error {
	if d.Version != Version || !idPattern.MatchString(d.Target) || len(d.Steps) == 0 || len(d.Steps) > 8 {
		return fmt.Errorf("invalid animation header")
	}
	total := 0
	for _, step := range d.Steps {
		total += step.DurationMS
		if step.DurationMS < 1 || step.DurationMS > 2000 || total > 5000 ||
			(step.Easing != "linear" && step.Easing != "easeOutCubic" && step.Easing != "easeInOutCubic") {
			return fmt.Errorf("invalid animation step")
		}
		for _, v := range []scene.Vec3{step.To.Position, step.To.Rotation, step.To.Scale} {
			for _, x := range v {
				if math.IsNaN(x) || math.IsInf(x, 0) {
					return fmt.Errorf("non-finite animation transform")
				}
			}
		}
		for _, x := range step.To.Scale {
			if x <= 0 {
				return fmt.Errorf("nonpositive animation scale")
			}
		}
	}
	return nil
}
