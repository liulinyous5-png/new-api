package doubao

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelListIncludesSeedance25(t *testing.T) {
	const model = "doubao-seedance-2-5-260628"
	for _, candidate := range ModelList {
		if candidate == model {
			return
		}
	}
	t.Fatalf("ModelList does not include %q", model)
}

func TestDreaminaModelList(t *testing.T) {
	want := []string{
		"dreamina-seedance-2-0-260128",
		"dreamina-seedance-2-0-fast-260128",
		"dreamina-seedance-2-0-mini-260615",
		"dreamina-seedance-2-5-260628",
	}

	for _, model := range want {
		assert.Contains(t, ModelList, model)
	}
}

func TestInternationalSeedanceModelList(t *testing.T) {
	for _, model := range []string{
		"seedance-1-0-pro-250528",
		"seedance-1-0-lite-t2v",
		"seedance-1-0-lite-i2v",
		"seedance-1-5-pro-251215",
	} {
		assert.Contains(t, ModelList, model)
	}
}

func TestDreaminaVideoInputRatio(t *testing.T) {
	tests := []struct {
		name       string
		model      string
		resolution string
		hasVideo   bool
		want       float64
	}{
		{name: "2.0 720p video input", model: "dreamina-seedance-2-0-260128", resolution: "720p", hasVideo: true, want: 4.3 / 7.0},
		{name: "2.0 1080p text input", model: "dreamina-seedance-2-0-260128", resolution: "1080p", want: 7.7 / 7.0},
		{name: "2.0 4k video input", model: "dreamina-seedance-2-0-260128", resolution: "4K", hasVideo: true, want: 2.4 / 7.0},
		{name: "2.0 fast video input", model: "dreamina-seedance-2-0-fast-260128", resolution: "720p", hasVideo: true, want: 3.3 / 5.6},
		{name: "2.0 mini video input", model: "dreamina-seedance-2-0-mini-260615", resolution: "480p", hasVideo: true, want: 2.1 / 3.5},
		{name: "2.5 1080p video input", model: "dreamina-seedance-2-5-260628", resolution: "1080p", hasVideo: true, want: 7.0 / 10.7},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := GetVideoInputRatio(test.model, test.resolution, test.hasVideo)
			require.True(t, ok)
			assert.InDelta(t, test.want, got, 1e-9)
		})
	}
}

func TestSeedance25VideoInputRatio(t *testing.T) {
	const model = "doubao-seedance-2-5-260628"
	tests := []struct {
		name     string
		hasVideo bool
		want     float64
	}{
		{name: "without video", hasVideo: false, want: 1.0},
		{name: "with video", hasVideo: true, want: 0.6},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := GetVideoInputRatio(model, "720p", test.hasVideo)
			if !ok {
				t.Fatal("expected Seedance 2.5 pricing configuration")
			}
			if math.Abs(got-test.want) > 1e-9 {
				t.Fatalf("GetVideoInputRatio() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestSeedance20MiniVideoInputRatio(t *testing.T) {
	const model = "doubao-seedance-2-0-mini-260615"
	found := false
	for _, candidate := range ModelList {
		if candidate == model {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ModelList does not include %q", model)
	}

	tests := []struct {
		name     string
		hasVideo bool
		want     float64
	}{
		{name: "without reference video", hasVideo: false, want: 1.0},
		{name: "with reference video", hasVideo: true, want: 14.0 / 23.0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := GetVideoInputRatio(model, "720p", test.hasVideo)
			if !ok {
				t.Fatal("expected Seedance 2.0 mini pricing configuration")
			}
			if math.Abs(got-test.want) > 1e-9 {
				t.Fatalf("GetVideoInputRatio() = %v, want %v", got, test.want)
			}
		})
	}
}
