package review

import (
	"testing"
	"time"
)

func TestParseCronExpr(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantH   int
		wantM   int
		wantDay *time.Weekday
		wantErr bool
	}{
		{name: "daily", expr: "daily", wantH: 0, wantM: 0},
		{name: "weekly", expr: "weekly", wantH: 0, wantM: 0, wantDay: weekdayPtr(time.Monday)},
		{name: "HH:MM", expr: "09:30", wantH: 9, wantM: 30},
		{name: "daily:HH:MM", expr: "daily:14:00", wantH: 14, wantM: 0},
		{name: "weekly:Fri", expr: "weekly:Fri", wantH: 0, wantM: 0, wantDay: weekdayPtr(time.Friday)},
		{name: "weekly:Wed:08:15", expr: "weekly:Wed:08:15", wantH: 8, wantM: 15, wantDay: weekdayPtr(time.Wednesday)},
		{name: "empty", expr: "", wantErr: true},
		{name: "invalid", expr: "every_hour", wantErr: true},
		{name: "bad hour", expr: "25:00", wantErr: true},
		{name: "bad minute", expr: "12:61", wantErr: true},
		{name: "bad weekday", expr: "weekly:Xyz", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sched, err := ParseCronExpr(tt.expr)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sched.Hour != tt.wantH {
				t.Errorf("hour: got %d, want %d", sched.Hour, tt.wantH)
			}
			if sched.Minute != tt.wantM {
				t.Errorf("minute: got %d, want %d", sched.Minute, tt.wantM)
			}
			if tt.wantDay == nil && sched.Weekday != nil {
				t.Errorf("weekday: got %v, want nil", *sched.Weekday)
			}
			if tt.wantDay != nil {
				if sched.Weekday == nil {
					t.Fatalf("weekday: got nil, want %v", *tt.wantDay)
				}
				if *sched.Weekday != *tt.wantDay {
					t.Errorf("weekday: got %v, want %v", *sched.Weekday, *tt.wantDay)
				}
			}
		})
	}
}

func TestCronScheduleNextAfter(t *testing.T) {
	// Wednesday 2026-02-25 at 10:30 UTC
	base := time.Date(2026, 2, 25, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name  string
		sched CronSchedule
		want  time.Time
	}{
		{
			name:  "daily before slot",
			sched: CronSchedule{Hour: 14, Minute: 0},
			want:  time.Date(2026, 2, 25, 14, 0, 0, 0, time.UTC),
		},
		{
			name:  "daily after slot",
			sched: CronSchedule{Hour: 9, Minute: 0},
			want:  time.Date(2026, 2, 26, 9, 0, 0, 0, time.UTC),
		},
		{
			name:  "weekly same day later",
			sched: CronSchedule{Hour: 14, Minute: 0, Weekday: weekdayPtr(time.Wednesday)},
			want:  time.Date(2026, 2, 25, 14, 0, 0, 0, time.UTC),
		},
		{
			name:  "weekly next occurrence",
			sched: CronSchedule{Hour: 9, Minute: 0, Weekday: weekdayPtr(time.Monday)},
			want:  time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC),
		},
		{
			name:  "weekly friday",
			sched: CronSchedule{Hour: 0, Minute: 0, Weekday: weekdayPtr(time.Friday)},
			want:  time.Date(2026, 2, 27, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.sched.NextAfter(base)
			if !got.Equal(tt.want) {
				t.Errorf("NextAfter: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateCronExpr(t *testing.T) {
	if err := ValidateCronExpr("daily"); err != nil {
		t.Errorf("expected valid: %v", err)
	}
	if err := ValidateCronExpr(""); err == nil {
		t.Error("expected error for empty string")
	}
}

func weekdayPtr(d time.Weekday) *time.Weekday {
	return &d
}
