package service

import (
	"github.com/sidneydekoning/analytics/internal/model"
)

// BuildFunnelResult converts raw step visitor counts into a FunnelResult with
// drop-off percentages.
func BuildFunnelResult(funnel *model.Funnel, steps []*model.FunnelStep, counts []int64) *model.FunnelResult {
	result := &model.FunnelResult{
		FunnelID:   funnel.ID,
		FunnelName: funnel.Name,
		Steps:      make([]model.FunnelStepResult, len(steps)),
	}
	for i, s := range steps {
		sr := model.FunnelStepResult{
			Position: s.Position,
			Name:     s.Name,
			Visitors: counts[i],
		}
		if i == 0 && counts[0] > 0 {
			sr.Converted = 100.0
		} else if counts[0] > 0 {
			sr.Converted = float64(counts[i]) / float64(counts[0]) * 100
			if i > 0 && counts[i-1] > 0 {
				sr.DropOff = (1.0 - float64(counts[i])/float64(counts[i-1])) * 100
			}
		}
		result.Steps[i] = sr
	}
	return result
}
