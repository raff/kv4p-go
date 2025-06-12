package widgets

import (
	"github.com/hajimehoshi/guigui"
)

type Audiovis interface {
	Update(context *guigui.Context, samples []int16, sampleRate int)

	guigui.Widget
}
