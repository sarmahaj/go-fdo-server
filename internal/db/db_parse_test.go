package db

import (
	"strings"
	"testing"
)

func TestListPendingTO0Vouchers_FilterByOnboarding(t *testing.T) {
	state, err := InitDb("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() {
		_ = state.Close()
	})

	guidPending := GUID([]byte{0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01})
	guidCompleted := GUID([]byte{0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02})
	guidNoRow := GUID([]byte{0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03})

	if err := db.Create(&Voucher{GUID: guidPending, DeviceInfo: "pending"}).Error; err != nil {
		t.Fatalf("create pending voucher: %v", err)
	}
	if err := db.Create(&Voucher{GUID: guidCompleted, DeviceInfo: "completed"}).Error; err != nil {
		t.Fatalf("create completed voucher: %v", err)
	}
	if err := db.Create(&Voucher{GUID: guidNoRow, DeviceInfo: "no-row"}).Error; err != nil {
		t.Fatalf("create no-row voucher: %v", err)
	}

	if err := db.Create(&DeviceOnboarding{GUID: guidPending, TO2Completed: false}).Error; err != nil {
		t.Fatalf("create pending onboarding: %v", err)
	}
	if err := db.Create(&DeviceOnboarding{GUID: guidCompleted, TO2Completed: true}).Error; err != nil {
		t.Fatalf("create completed onboarding: %v", err)
	}

	vouchers, err := ListPendingTO0Vouchers(false)
	if err != nil {
		t.Fatalf("list pending vouchers: %v", err)
	}

	if len(vouchers) != 1 {
		t.Fatalf("expected 1 pending voucher, got %d", len(vouchers))
	}
	if string(vouchers[0].GUID) != string(guidPending) {
		t.Fatalf("expected pending guid %x, got %x", guidPending, vouchers[0].GUID)
	}
}

func TestParsePortValueV1_Cases(t *testing.T) {
	cases := []struct {
		name      string
		in        any
		want      uint16
		wantError bool
		errSubstr string
	}{
		// String inputs (V1 API backward compatibility)
		{name: "string_valid_min", in: "1", want: 1},
		{name: "string_valid_max", in: "65535", want: 65535},
		{name: "string_invalid_zero", in: "0", wantError: true, errSubstr: "port out of range"},
		{name: "string_invalid_out_of_range", in: "65536", wantError: true, errSubstr: "port out of range"},
		{name: "string_invalid_non_numeric", in: "eighty", wantError: true, errSubstr: "invalid syntax"},
		{name: "string_invalid_empty", in: "", wantError: true, errSubstr: "empty port"},

		// Integer inputs (also supported in V1)
		{name: "float_valid_lower_bound", in: float64(1), want: 1},
		{name: "float_valid_upper_bound", in: float64(65535), want: 65535},
		{name: "float_invalid_fractional", in: 8043.5, wantError: true, errSubstr: "port must be an integer"},
		{name: "float_invalid_below_min", in: float64(0), wantError: true, errSubstr: "port out of range"},
		{name: "float_invalid_above_max", in: float64(65536), wantError: true, errSubstr: "port out of range"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePortValueV1(tc.in)
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Fatalf("expected error containing %q, got %q", tc.errSubstr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("unexpected value: got %d want %d", got, tc.want)
			}
		})
	}
}
