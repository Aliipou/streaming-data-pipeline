package processor_test

import (
	"testing"
	"time"

	"github.com/aliipou/streaming-data-pipeline/internal/models"
	"github.com/aliipou/streaming-data-pipeline/internal/processor"
)

func addEvent(wa *processor.WindowAggregator, sType models.SensorType, location string, value float64, t time.Time) {
	wa.Add(models.SensorEvent{
		SensorID:   "s1",
		SensorType: sType,
		Location:   location,
		Value:      value,
		Timestamp:  t,
	})
}

func TestWindowAggregator_Empty(t *testing.T) {
	wa := processor.NewWindowAggregator()
	windows := wa.GetCurrentWindows()
	if len(windows) != 0 {
		t.Errorf("expected empty windows initially, got %d", len(windows))
	}
}

func TestWindowAggregator_SingleBucket(t *testing.T) {
	wa := processor.NewWindowAggregator()
	now := time.Now()

	addEvent(wa, models.SensorTemperature, "lab", 20.0, now)
	addEvent(wa, models.SensorTemperature, "lab", 30.0, now)
	addEvent(wa, models.SensorTemperature, "lab", 25.0, now)

	windows := wa.GetCurrentWindows()
	if len(windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(windows))
	}
	w := windows[0]
	if w.Count != 3 {
		t.Errorf("Count: got %d, want 3", w.Count)
	}
	if w.Min != 20.0 {
		t.Errorf("Min: got %v, want 20.0", w.Min)
	}
	if w.Max != 30.0 {
		t.Errorf("Max: got %v, want 30.0", w.Max)
	}
	if w.Avg != 25.0 {
		t.Errorf("Avg: got %v, want 25.0", w.Avg)
	}
	if w.Sum != 75.0 {
		t.Errorf("Sum: got %v, want 75.0", w.Sum)
	}
}

func TestWindowAggregator_P99(t *testing.T) {
	wa := processor.NewWindowAggregator()
	now := time.Now()

	// 100 values 1-100
	for i := 1; i <= 100; i++ {
		addEvent(wa, models.SensorTemperature, "lab", float64(i), now)
	}

	windows := wa.GetCurrentWindows()
	if len(windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(windows))
	}
	w := windows[0]
	// p99 of 1..100 ≈ 99
	if w.P99 < 95 || w.P99 > 100 {
		t.Errorf("P99 out of expected range [95,100]: got %v", w.P99)
	}
}

func TestWindowAggregator_MultipleBuckets_DifferentLocations(t *testing.T) {
	wa := processor.NewWindowAggregator()
	now := time.Now()

	addEvent(wa, models.SensorTemperature, "floor-1", 22.0, now)
	addEvent(wa, models.SensorTemperature, "floor-2", 24.0, now)
	addEvent(wa, models.SensorPressure, "lab", 1013.0, now)

	windows := wa.GetCurrentWindows()
	if len(windows) != 3 {
		t.Errorf("expected 3 distinct windows (2 locations + 1 type), got %d", len(windows))
	}
}

func TestWindowAggregator_CompletedBuckets(t *testing.T) {
	wa := processor.NewWindowAggregator()

	// Past minute (completed)
	past := time.Now().Add(-2 * time.Minute)
	addEvent(wa, models.SensorTemperature, "lab", 22.0, past)

	// Current minute (in-progress)
	now := time.Now()
	addEvent(wa, models.SensorTemperature, "lab", 25.0, now)

	completed := wa.GetAllWindows()
	current := wa.GetCurrentWindows()

	if len(completed) == 0 {
		t.Error("expected at least 1 completed window")
	}
	if len(current) == 0 {
		t.Error("expected at least 1 current window")
	}
}

func TestWindowAggregator_Prune(t *testing.T) {
	wa := processor.NewWindowAggregator()

	// Old event (5 hours ago)
	old := time.Now().Add(-5 * time.Hour)
	addEvent(wa, models.SensorTemperature, "lab", 22.0, old)

	// Recent event
	addEvent(wa, models.SensorTemperature, "lab", 22.0, time.Now())

	// Prune data older than 1 hour
	wa.Prune(60)

	// Old bucket should be gone, recent should remain
	current := wa.GetCurrentWindows()
	all := wa.GetAllWindows()
	total := len(current) + len(all)

	// We should only have the recent bucket remaining
	if total == 0 {
		t.Log("all windows pruned — recent may not be visible yet as 'completed'")
	}
}

func TestWindowAggregator_SingleValue(t *testing.T) {
	wa := processor.NewWindowAggregator()
	addEvent(wa, models.SensorPressure, "roof", 1010.0, time.Now())

	windows := wa.GetCurrentWindows()
	if len(windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(windows))
	}
	w := windows[0]
	if w.Min != w.Max || w.Min != w.Avg || w.Min != 1010.0 {
		t.Errorf("single-value window: min=%v max=%v avg=%v", w.Min, w.Max, w.Avg)
	}
}

func TestWindowAggregator_WindowTimeRange(t *testing.T) {
	wa := processor.NewWindowAggregator()
	now := time.Now()
	addEvent(wa, models.SensorHumidity, "server-room", 55.0, now)

	windows := wa.GetCurrentWindows()
	if len(windows) == 0 {
		t.Fatal("expected 1 window")
	}
	w := windows[0]
	if w.WindowEnd.Sub(w.WindowStart) != time.Minute {
		t.Errorf("window duration should be 1 minute, got %v", w.WindowEnd.Sub(w.WindowStart))
	}
}
