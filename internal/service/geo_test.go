package service_test

import (
	"testing"

	"github.com/funky-monkey/analyics-dash-tics/internal/service"
	"github.com/stretchr/testify/assert"
)

func TestGeoLocator_ReturnsEmptyWhenNoDB(t *testing.T) {
	geo := service.NewGeoLocator("")
	country, city := geo.Lookup("203.0.113.42")
	assert.Equal(t, "", country)
	assert.Equal(t, "", city)
}

func TestGeoLocator_HandlesLoopback(t *testing.T) {
	geo := service.NewGeoLocator("")
	country, city := geo.Lookup("127.0.0.1")
	assert.Equal(t, "", country)
	assert.Equal(t, "", city)
}

func TestGeoLocator_HandlesPrivateIP(t *testing.T) {
	geo := service.NewGeoLocator("")
	country, city := geo.Lookup("192.168.1.100")
	assert.Equal(t, "", country)
	assert.Equal(t, "", city)
}
