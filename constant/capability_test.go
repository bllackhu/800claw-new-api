package constant

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCapabilityByPath(t *testing.T) {
	assert.Equal(t, CapabilitySpeechRecognition, GetCapabilityByPath("/v1/audio/transcriptions"))
	assert.Equal(t, CapabilitySpeechRecognition, GetCapabilityByPath("/v1/audio/transcriptions?foo=bar"))
	assert.Equal(t, "", GetCapabilityByPath("/v1/chat/completions"))
	assert.Equal(t, "", GetCapabilityByPath("/v1/images/generations"))
	assert.Equal(t, "", GetCapabilityByPath("/"))
}

func TestIsValidCapability(t *testing.T) {
	assert.True(t, IsValidCapability(CapabilitySpeechRecognition))
	assert.False(t, IsValidCapability("web_search"))
	assert.False(t, IsValidCapability(""))
}

func TestGetCapabilityUnit(t *testing.T) {
	assert.Equal(t, CapabilityUnitMinutes, GetCapabilityUnit(CapabilitySpeechRecognition))
	assert.Equal(t, CapabilityUnitCount, GetCapabilityUnit("unknown_capability"))
}

func TestCapabilityRegistryHasSpeechRecognition(t *testing.T) {
	meta := GetCapabilityMeta(CapabilitySpeechRecognition)
	assert.NotNil(t, meta)
	assert.Equal(t, CapabilitySpeechRecognition, meta.Name)
	assert.Contains(t, meta.Paths, "/v1/audio/transcriptions")
}
