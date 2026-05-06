package service_test

import (
	"testing"
	"time"

	"github.com/sidneydekoning/analytics/internal/service"
	"github.com/stretchr/testify/assert"
)

func TestDateRange_Last30Days(t *testing.T) {
	from, to := service.DateRange("30d")
	diff := to.Sub(from)
	assert.InDelta(t, 30*24*float64(time.Hour), float64(diff), float64(time.Hour))
	assert.True(t, to.After(from))
}

func TestDateRange_Last7Days(t *testing.T) {
	from, to := service.DateRange("7d")
	diff := to.Sub(from)
	assert.InDelta(t, 7*24*float64(time.Hour), float64(diff), float64(time.Hour))
}

func TestDateRange_Today(t *testing.T) {
	from, to := service.DateRange("today")
	assert.Equal(t, from.YearDay(), to.YearDay())
}

func TestDateRange_Unknown_Defaults30Days(t *testing.T) {
	from, to := service.DateRange("invalid")
	diff := to.Sub(from)
	assert.InDelta(t, 30*24*float64(time.Hour), float64(diff), float64(time.Hour))
}
