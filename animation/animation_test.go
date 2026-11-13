package animation

import (
	"github.com/n0remac/trick-evolution/scene"
	"testing"
)

func TestDescriptionValidation(t *testing.T) {
	d := New("card_H_02", Step{To: scene.Identity(), DurationMS: 180, Easing: "easeOutCubic"})
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	d.Steps[0].Easing = "script"
	if d.Validate() == nil {
		t.Fatal("accepted arbitrary easing")
	}
	d.Steps[0].Easing = "linear"
	d.Steps[0].DurationMS = 6000
	if d.Validate() == nil {
		t.Fatal("accepted unbounded animation")
	}
}
