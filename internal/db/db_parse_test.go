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

func TestParseHumanToTO2AddrsJSON_Cases(t *testing.T) {
	cases := []struct {
		name      string
		jsonBody  string
		wantError bool
		errSubstr string
	}{
		{
			name:     "valid_dns_only",
			jsonBody: `[{"dns":"owner.example.com","port":"32768","protocol":"http"}]`,
		},
		{
			name:     "valid_ip_only",
			jsonBody: `[{"ip":"10.0.0.5","port":"32768","protocol":"https"}]`,
		},
		{
			name:     "valid_both",
			jsonBody: `[{"dns":"owner.example.com","ip":"10.0.0.5","port":"32768","protocol":"tls"}]`,
		},
		{
			name:      "invalid_missing_dns_ip",
			jsonBody:  `[{}]`,
			wantError: true,
			errSubstr: "at least one of dns or ip",
		},
		{
			name:      "invalid_protocol",
			jsonBody:  `[{"dns":"owner.example.com","port":"32768","protocol":"bogus"}]`,
			wantError: true,
			errSubstr: "unsupported transport protocol",
		},
		{
			name:      "invalid_port_non_numeric",
			jsonBody:  `[{"dns":"owner.example.com","port":"eightythree","protocol":"http"}]`,
			wantError: true,
			errSubstr: "port:",
		},
		{
			name:      "invalid_transport_protocol_case",
			jsonBody:  `[{"dns":"owner.example.com","port":"32768","protocol":"HTTP"}]`,
			wantError: true,
			errSubstr: "unsupported transport protocol",
		},
		{
			name:      "invalid_json_malformed",
			jsonBody:  `[{bad}]`,
			wantError: true,
			errSubstr: "invalid",
		},
		{
			name:      "invalid_top_level_object",
			jsonBody:  `{}`,
			wantError: true,
			errSubstr: "cannot unmarshal object",
		},
		{
			name:      "invalid_protocol_type_number",
			jsonBody:  `[{"dns":"owner.example.com","port":8043,"protocol":1}]`,
			wantError: true,
			errSubstr: "cannot unmarshal number",
		},
		{
			name:     "port_empty_is_ignored",
			jsonBody: `[{"dns":"owner.example.com","port":"","protocol":"http"}]`,
		},
		{
			name:      "invalid_port_float",
			jsonBody:  `[{"dns":"owner.example.com","port":8043.5,"protocol":"http"}]`,
			wantError: true,
			errSubstr: "cannot unmarshal number",
		},
		{
			name:      "invalid_dns_type_number",
			jsonBody:  `[{"dns":123,"port":8043,"protocol":"http"}]`,
			wantError: true,
			errSubstr: "cannot unmarshal number",
		},
		{
			name:      "invalid_ip_type_number",
			jsonBody:  `[{"ip":123,"port":8043,"protocol":"http"}]`,
			wantError: true,
			errSubstr: "cannot unmarshal number",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseHumanToTO2AddrsJSON([]byte(tc.jsonBody))
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
		})
	}
}

func TestParsePortValue_Cases(t *testing.T) {
	cases := []struct {
		name      string
		in        any
		want      uint16
		wantError bool
		errSubstr string
	}{
		// String inputs are no longer supported (OpenAPI requires integer)
		{name: "string_rejected", in: "1", wantError: true, errSubstr: "port must be an integer"},
		{name: "string_rejected_large", in: "65535", wantError: true, errSubstr: "port must be an integer"},
		{name: "string_rejected_invalid", in: "0", wantError: true, errSubstr: "port must be an integer"},
		{name: "string_rejected_out_of_range", in: "65536", wantError: true, errSubstr: "port must be an integer"},
		{name: "string_rejected_non_numeric", in: "eighty", wantError: true, errSubstr: "port must be an integer"},
		{name: "string_rejected_empty", in: "", wantError: true, errSubstr: "port must be an integer"},

		// Integer inputs (float64 in JSON)
		{name: "float_valid_lower_bound", in: float64(1), want: 1},
		{name: "float_valid_upper_bound", in: float64(65535), want: 65535},
		{name: "float_invalid_fractional", in: 8043.5, wantError: true, errSubstr: "whole number"},
		{name: "float_invalid_below_min", in: float64(0), wantError: true, errSubstr: "between 1 and 65535"},
		{name: "float_invalid_above_max", in: float64(65536), wantError: true, errSubstr: "between 1 and 65535"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePortValue(tc.in)
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
