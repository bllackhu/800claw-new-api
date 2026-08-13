package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func newTestGinContextWithUnit(unit string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if unit != "" {
		c.Set(string(constant.ContextKeyCapabilityUnit), unit)
	}
	return c
}

func TestCapabilityUnitsFromDuration_Minutes(t *testing.T) {
	c := newTestGinContextWithUnit(constant.CapabilityUnitMinutes)

	// 2499ms => ceil(2.499/60) = 1
	assert.Equal(t, int64(1), CapabilityUnitsFromDuration(c, 2499))
	// 60000ms => 1
	assert.Equal(t, int64(1), CapabilityUnitsFromDuration(c, 60000))
	// 61000ms => ceil(61/60) = 2
	assert.Equal(t, int64(2), CapabilityUnitsFromDuration(c, 61000))
	// 0 => fallback 1
	assert.Equal(t, int64(1), CapabilityUnitsFromDuration(c, 0))
	// negative => fallback 1
	assert.Equal(t, int64(1), CapabilityUnitsFromDuration(c, -5))
}

func TestCapabilityUnitsFromDuration_Seconds(t *testing.T) {
	c := newTestGinContextWithUnit(constant.CapabilityUnitSeconds)
	assert.Equal(t, int64(1), CapabilityUnitsFromDuration(c, 1000))
	assert.Equal(t, int64(2), CapabilityUnitsFromDuration(c, 1001))
	// 0 时长回退为最小计量 1
	assert.Equal(t, int64(1), CapabilityUnitsFromDuration(c, 0))
}

func TestCapabilityUnitsFromDuration_Count(t *testing.T) {
	// count 单位忽略时长，恒为 1
	c := newTestGinContextWithUnit(constant.CapabilityUnitCount)
	assert.Equal(t, int64(1), CapabilityUnitsFromDuration(c, 5000))
	assert.Equal(t, int64(1), CapabilityUnitsFromDuration(c, 0))
}

func TestIsCapabilityRequest(t *testing.T) {
	c := newTestGinContextWithUnit("")
	assert.False(t, IsCapabilityRequest(c))
	c.Set(string(constant.ContextKeyCapability), constant.CapabilitySpeechRecognition)
	assert.True(t, IsCapabilityRequest(c))
}
