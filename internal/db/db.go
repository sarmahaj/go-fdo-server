// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package db

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"

	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var db *gorm.DB

// Sentinel errors to classify client input issues
var (
	ErrInvalidOwnerInfo = errors.New("invalid ownerinfo data")
	ErrInvalidRvInfo    = errors.New("invalid rvinfo data")
)

// FetchVoucher returns a single voucher filtered by provided fields.
// Supported filters (keys):
// - "guid" (expects []byte)
// - "device_info" (expects string)
// If more than one voucher matches, an error is returned.
func FetchVoucher(filters map[string]interface{}) (*Voucher, error) {
	if len(filters) == 0 {
		return nil, fmt.Errorf("no filters provided")
	}
	list, err := QueryVouchers(filters, true)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if len(list) > 1 {
		return nil, fmt.Errorf("multiple vouchers matched filters")
	}
	return &list[0], nil
}

// QueryVouchers returns owner vouchers matching optional filters.
// If includeCBOR is true, the CBOR column is selected and populated.
// Results are ordered by updated_at DESC.
func QueryVouchers(filters map[string]interface{}, includeCBOR bool) ([]Voucher, error) {
	query := db.Model(&Voucher{})

	// Apply filters
	if v, ok := filters["guid"]; ok {
		b, ok := v.([]byte)
		if !ok {
			return nil, fmt.Errorf("invalid type for guid filter; want []byte")
		}
		query = query.Where("guid = ?", b)
	}
	if v, ok := filters["device_info"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("invalid type for device_info filter; want string")
		}
		query = query.Where("device_info = ?", s)
	}

	// Omit CBOR if not needed
	if !includeCBOR {
		query = query.Omit("cbor")
	}

	// Order by updated_at DESC
	query = query.Order("updated_at DESC")

	var list []Voucher
	if err := query.Find(&list).Error; err != nil {
		return nil, err
	}

	return list, nil
}

func InsertVoucher(voucher Voucher) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&voucher).Error; err != nil {
			return err
		}
		// Ensure onboarding tracking row exists atomically with voucher insert
		rec := DeviceOnboarding{GUID: voucher.GUID}
		return tx.Where("guid = ?", voucher.GUID).FirstOrCreate(&rec).Error
	})
}

// IsTO2Completed returns whether a device has completed TO2.
func IsTO2Completed(guid []byte) (bool, error) {
	var rec DeviceOnboarding
	if err := db.Where("guid = ?", guid).First(&rec).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, err
	}
	return rec.TO2Completed, nil
}

// ListPendingTO0Vouchers returns vouchers whose devices have not completed TO2 yet.
// A voucher is considered pending if TO2Completed is false.
func ListPendingTO0Vouchers(includeCBOR bool) ([]Voucher, error) {
	query := db.Model(&Voucher{})
	// Join with device_onboarding to filter by completion state
	query = query.Joins("LEFT JOIN device_onboarding ON device_onboarding.guid = vouchers.guid").
		Where("device_onboarding.to2_completed = ?", false)

	if !includeCBOR {
		query = query.Omit("cbor")
	}

	var list []Voucher
	if err := query.Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func InsertOwnerInfo(data []byte) error {
	// check the data can be parsed into []protocol.RvTO2Addr
	if _, err := parseHumanToTO2AddrsJSON(data); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidOwnerInfo, err)
	}

	ownerInfo := OwnerInfo{
		ID:    1,
		Value: data,
	}
	tx := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&ownerInfo)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrDuplicatedKey
	}
	return nil
}

func UpdateOwnerInfo(data []byte) error {
	// check the data can be parsed into []protocol.RvTO2Addr
	if _, err := parseHumanToTO2AddrsJSON(data); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidOwnerInfo, err)
	}

	tx := db.Model(&OwnerInfo{}).Where("id = ?", 1).Update("value", data)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func FetchOwnerInfoJSON() ([]byte, error) {
	var ownerInfo OwnerInfo
	if err := db.Where("id = ?", 1).First(&ownerInfo).Error; err != nil {
		return nil, err
	}
	return ownerInfo.Value, nil
}

// FetchOwnerInfoData reads the owner_info JSON (stored as text) and converts it
// into []protocol.RvTO2Addr.
func FetchOwnerInfo() ([]protocol.RvTO2Addr, error) {
	ownerInfoData, err := FetchOwnerInfoJSON()
	if err != nil {
		return nil, err
	}

	return parseHumanToTO2AddrsJSON(ownerInfoData)
}

func InsertRvInfo(data []byte) error {
	// check the data can be parsed into [][]protocol.RvInstruction
	if _, err := ParseOpenAPIRvJSON(data); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRvInfo, err)
	}

	rvInfo := RvInfo{
		ID:    1,
		Value: data,
	}
	tx := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&rvInfo)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrDuplicatedKey
	}
	return nil
}

func UpdateRvInfo(data []byte) error {
	// check the data can be parsed into [][]protocol.RvInstruction
	if _, err := ParseOpenAPIRvJSON(data); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRvInfo, err)
	}

	tx := db.Model(&RvInfo{}).Where("id = ?", 1).Update("value", data)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func FetchRvInfoJSON() ([]byte, error) {
	var rvInfo RvInfo
	if err := db.Where("id = ?", 1).First(&rvInfo).Error; err != nil {
		return nil, err
	}
	return rvInfo.Value, nil
}

func DeleteRvInfo() error {
	tx := db.Where("id = ?", 1).Delete(&RvInfo{})
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ListDevices returns devices known to the owner service, combining voucher
// metadata with TO2 onboarding state (if any) from device_onboarding.
// Devices are ordered by most recently updated voucher first.
func ListDevices(filters map[string]interface{}) ([]Device, error) {
	var out []Device

	query := db.Table("vouchers").
		Select("vouchers.guid, device_onboarding.guid as old_guid, vouchers.device_info, vouchers.created_at, vouchers.updated_at, device_onboarding.to2_completed, device_onboarding.to2_completed_at").
		Joins("LEFT JOIN device_onboarding ON device_onboarding.new_guid = vouchers.guid").
		Order("vouchers.updated_at DESC")

	// Apply filters
	if v, ok := filters["old_guid"]; ok {
		b, ok := v.([]byte)
		if !ok {
			return nil, fmt.Errorf("invalid type for old_guid filter; want []byte")
		}
		query = query.Where("device_onboarding.guid = ?", b)
	}

	if err := query.Scan(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// FetchRvInfo reads the rvinfo JSON (stored as text) and converts it into
// [][]protocol.RvInstruction, CBOR-encoding each value as required by go-fdo.
func FetchRvInfo() ([][]protocol.RvInstruction, error) {
	rvInfo, err := FetchRvInfoJSON()
	if err != nil {
		return nil, err
	}
	return ParseOpenAPIRvJSON(rvInfo)
}

func encodeRvValue(rvVar protocol.RvVar, val any) ([]byte, error) {
	switch v := val.(type) {
	case string:
		switch rvVar {
		case protocol.RVDns:
			return cbor.Marshal(v)
		case protocol.RVIPAddress:
			ip := net.ParseIP(v)
			if ip == nil {
				return nil, fmt.Errorf("invalid ip %q", v)
			}
			return cbor.Marshal(ip)
		default:
			return cbor.Marshal(v)
		}
	case bool:
		return cbor.Marshal(v)
	case float64:
		// JSON numbers -> coerce by variable semantics
		switch rvVar {
		case protocol.RVDevPort, protocol.RVOwnerPort:
			return cbor.Marshal(uint16(v))
		case protocol.RVProtocol, protocol.RVMedium:
			return cbor.Marshal(uint8(v))
		case protocol.RVDelaysec:
			return cbor.Marshal(uint32(v))
		default:
			return cbor.Marshal(int64(v))
		}
	default:
		return cbor.Marshal(v)
	}
}

// ParseOpenAPIRvJSON parses OpenAPI-formatted RvInfo JSON into [][]protocol.RvInstruction.
//
// Expected format (array of arrays of single-key objects):
//
//	[
//	  [
//	    {"dns": "rendezvous.example.com"},
//	    {"protocol": "http"},
//	    {"owner_port": 8080}
//	  ],
//	  [
//	    {"ip": "192.168.1.100"},
//	    {"protocol": "https"},
//	    {"owner_port": 8443}
//	  ]
//	]
//
// Each outer array element is an RV directive (fallback options).
// Each inner array element is a single instruction (key-value pair).
// This format aligns with the OpenAPI specification and FDO protocol CBOR structure.
// ParseOpenAPIRvJSON parses OpenAPI format RV info JSON into protocol instructions
func ParseOpenAPIRvJSON(rawJSON []byte) ([][]protocol.RvInstruction, error) {
	// Parse as array of arrays of single-key objects
	var directives [][]map[string]interface{}
	if err := json.Unmarshal(rawJSON, &directives); err != nil {
		return nil, fmt.Errorf("invalid rvinfo JSON (expected array of arrays): %w", err)
	}

	out := make([][]protocol.RvInstruction, 0, len(directives))

	for directiveIdx, instructions := range directives {
		group := make([]protocol.RvInstruction, 0, len(instructions))
		hasDNSorIP := false

		for instrIdx, instruction := range instructions {
			// Each instruction should be a single-key object
			if len(instruction) != 1 {
				return nil, fmt.Errorf("rvinfo[%d][%d]: each instruction must be a single-key object, got %d keys",
					directiveIdx, instrIdx, len(instruction))
			}

			// Extract the single key-value pair
			var key string
			var value interface{}
			for k, v := range instruction {
				key = k
				value = v
				break
			}

			// Map key to protocol instruction and encode value
			rvInstr, err := parseRvInstruction(key, value)
			if err != nil {
				return nil, fmt.Errorf("rvinfo[%d][%d]: %w", directiveIdx, instrIdx, err)
			}

			// Track if we have DNS or IP
			if rvInstr.Variable == protocol.RVDns || rvInstr.Variable == protocol.RVIPAddress {
				hasDNSorIP = true
			}

			group = append(group, *rvInstr)
		}

		// Spec requires at least one of DNS or IP to be present for an RV entry
		if !hasDNSorIP {
			return nil, fmt.Errorf("rvinfo[%d]: at least one of dns or ip must be specified", directiveIdx)
		}

		out = append(out, group)
	}

	return out, nil
}

// parseRvInstruction converts a single key-value pair into a protocol.RvInstruction
func parseRvInstruction(key string, value interface{}) (*protocol.RvInstruction, error) {
	switch key {
	case "dns":
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("dns must be a string, got %T", value)
		}
		enc, err := encodeRvValue(protocol.RVDns, str)
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVDns, Value: enc}, nil

	case "ip":
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("ip must be a string, got %T", value)
		}
		enc, err := encodeRvValue(protocol.RVIPAddress, str)
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVIPAddress, Value: enc}, nil

	case "protocol":
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("protocol must be a string, got %T", value)
		}
		code, err := protocolCodeFromString(str)
		if err != nil {
			return nil, err
		}
		enc, err := encodeRvValue(protocol.RVProtocol, uint8(code))
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVProtocol, Value: enc}, nil

	case "medium":
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("medium must be a string, got %T", value)
		}
		m, err := parseMediumValue(str)
		if err != nil {
			return nil, err
		}
		enc, err := encodeRvValue(protocol.RVMedium, uint8(m))
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVMedium, Value: enc}, nil

	case "device_port":
		num, err := parsePortValueV2(value)
		if err != nil {
			return nil, fmt.Errorf("device_port: %w", err)
		}
		enc, err := encodeRvValue(protocol.RVDevPort, num)
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVDevPort, Value: enc}, nil

	case "owner_port":
		num, err := parsePortValueV2(value)
		if err != nil {
			return nil, fmt.Errorf("owner_port: %w", err)
		}
		enc, err := encodeRvValue(protocol.RVOwnerPort, num)
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVOwnerPort, Value: enc}, nil

	case "wifi_ssid":
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("wifi_ssid must be a string, got %T", value)
		}
		enc, err := encodeRvValue(protocol.RVWifiSsid, str)
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVWifiSsid, Value: enc}, nil

	case "wifi_pw":
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("wifi_pw must be a string, got %T", value)
		}
		enc, err := encodeRvValue(protocol.RVWifiPw, str)
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVWifiPw, Value: enc}, nil

	case "dev_only":
		b, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("dev_only must be a boolean, got %T", value)
		}
		if b {
			return &protocol.RvInstruction{Variable: protocol.RVDevOnly}, nil
		}
		return nil, fmt.Errorf("dev_only is false, should be omitted instead")

	case "owner_only":
		b, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("owner_only must be a boolean, got %T", value)
		}
		if b {
			return &protocol.RvInstruction{Variable: protocol.RVOwnerOnly}, nil
		}
		return nil, fmt.Errorf("owner_only is false, should be omitted instead")

	case "rv_bypass":
		b, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("rv_bypass must be a boolean, got %T", value)
		}
		if b {
			return &protocol.RvInstruction{Variable: protocol.RVBypass}, nil
		}
		return nil, fmt.Errorf("rv_bypass is false, should be omitted instead")

	case "delay_seconds":
		// OpenAPI spec requires integer type
		num, ok := value.(float64)
		if !ok {
			return nil, fmt.Errorf("delay_seconds must be an integer, got %T", value)
		}

		// Ensure it's a whole number
		if num != math.Trunc(num) {
			return nil, fmt.Errorf("delay_seconds must be a whole number, got %v", num)
		}

		// Validate non-negative
		if num < 0 {
			return nil, fmt.Errorf("delay_seconds must be non-negative, got %v", num)
		}

		secs := uint32(num)
		enc, err := encodeRvValue(protocol.RVDelaysec, uint64(secs))
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVDelaysec, Value: enc}, nil

	case "sv_cert_hash":
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("sv_cert_hash must be a string, got %T", value)
		}
		b, err := hex.DecodeString(str)
		if err != nil {
			return nil, fmt.Errorf("sv_cert_hash: %w", err)
		}
		enc, err := cbor.Marshal(b)
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVSvCertHash, Value: enc}, nil

	case "cl_cert_hash":
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("cl_cert_hash must be a string, got %T", value)
		}
		b, err := hex.DecodeString(str)
		if err != nil {
			return nil, fmt.Errorf("cl_cert_hash: %w", err)
		}
		enc, err := cbor.Marshal(b)
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVClCertHash, Value: enc}, nil

	case "user_input":
		b, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("user_input must be a boolean, got %T", value)
		}
		if b {
			// Only include instruction if true (per FDO spec)
			return &protocol.RvInstruction{Variable: protocol.RVUserInput}, nil
		}
		return nil, fmt.Errorf("user_input is false, should be omitted instead")

	case "ext_rv":
		arr, ok := value.([]interface{})
		if !ok {
			return nil, fmt.Errorf("ext_rv must be an array, got %T", value)
		}
		// Convert []interface{} to []string
		strArr := make([]string, len(arr))
		for i, v := range arr {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("ext_rv[%d] must be a string, got %T", i, v)
			}
			strArr[i] = s
		}
		enc, err := encodeRvValue(protocol.RVExtRV, strArr)
		if err != nil {
			return nil, err
		}
		return &protocol.RvInstruction{Variable: protocol.RVExtRV, Value: enc}, nil

	default:
		return nil, fmt.Errorf("unknown instruction key: %q", key)
	}
}

// parsePortValueV2 validates port values for V2 API (OpenAPI spec).
// Only accepts integers (float64 in JSON). Rejects strings for strict type compliance.
func parsePortValueV2(v any) (uint16, error) {
	// Only accept integer (float64 in JSON)
	f, ok := v.(float64)
	if !ok {
		return 0, fmt.Errorf("port must be an integer, got %T", v)
	}

	// Ensure it's a whole number (not 8080.5)
	if f != math.Trunc(f) {
		return 0, fmt.Errorf("port must be a whole number, got %v", f)
	}

	// Validate port range (1-65535)
	if f < 1 || f > 65535 {
		return 0, fmt.Errorf("port must be between 1 and 65535, got %v", f)
	}

	return uint16(f), nil
}

// parsePortValueV1 validates port values for V1 API (backward compatibility).
// Accepts both integers and strings for legacy support.
func parsePortValueV1(v any) (uint16, error) {
	switch t := v.(type) {
	case float64:
		if t != math.Trunc(t) {
			return 0, fmt.Errorf("port must be an integer, got %v", t)
		}
		if t < 1 || t > 65535 {
			return 0, fmt.Errorf("port out of range: %v", t)
		}
		return uint16(t), nil
	case string:
		// V1 API backward compatibility: accept string ports
		if t == "" {
			return 0, fmt.Errorf("empty port")
		}
		i, err := strconv.Atoi(t)
		if err != nil {
			return 0, err
		}
		if i < 1 || i > 65535 {
			return 0, fmt.Errorf("port out of range: %d", i)
		}
		return uint16(i), nil
	default:
		return 0, fmt.Errorf("port must be an integer or string, got %T", v)
	}
}

func protocolCodeFromString(s string) (uint8, error) {
	switch s {
	case "rest":
		return uint8(protocol.RVProtRest), nil
	case "http":
		return uint8(protocol.RVProtHTTP), nil
	case "https":
		return uint8(protocol.RVProtHTTPS), nil
	case "tcp":
		return uint8(protocol.RVProtTCP), nil
	case "tls":
		return uint8(protocol.RVProtTLS), nil
	case "coap+tcp":
		return uint8(protocol.RVProtCoapTCP), nil
	case "coap":
		return uint8(protocol.RVProtCoapUDP), nil
	default:
		return 0, fmt.Errorf("unsupported protocol %q", s)
	}
}

func parseMediumValue(v any) (uint8, error) {
	switch t := v.(type) {
	case float64:
		return uint8(t), nil
	case string:
		switch t {
		case "eth_all":
			return protocol.RVMedEthAll, nil
		case "wifi_all":
			return protocol.RVMedWifiAll, nil
		default:
			return 0, fmt.Errorf("unsupported medium %q", t)
		}
	default:
		return 0, fmt.Errorf("unsupported medium type %T", v)
	}
}

// ParseHumanToTO2AddrsJSON parses a JSON like
// [{"dns":"fdo.example.com","port":"8082","protocol":"http","ip":"127.0.0.1"}]
// into []protocol.RvTO2Addr.
func parseHumanToTO2AddrsJSON(rawJSON []byte) ([]protocol.RvTO2Addr, error) {
	// Strongly-typed JSON for validation
	type to2Human struct {
		DNS      string `json:"dns"`
		IP       string `json:"ip"`
		Port     string `json:"port"`
		Protocol string `json:"protocol"`
	}
	var items []to2Human
	if err := json.Unmarshal(rawJSON, &items); err != nil {
		return nil, fmt.Errorf("invalid TO2 addrs JSON: %w", err)
	}

	out := make([]protocol.RvTO2Addr, 0, len(items))
	for i, item := range items {
		var (
			ipPtr  *net.IP
			dnsPtr *string
			port   uint16
			proto  protocol.TransportProtocol
		)

		if item.IP != "" {
			ip := net.ParseIP(item.IP)
			if ip == nil {
				return nil, fmt.Errorf("invalid ip %q", item.IP)
			}
			ipPtr = &ip
		}
		if item.DNS != "" {
			dns := item.DNS
			dnsPtr = &dns
		}
		// Spec: A given RVTO2Addr must have at least one of RVIP or RVDNS
		if ipPtr == nil && dnsPtr == nil {
			return nil, fmt.Errorf("to2[%d]: at least one of dns or ip must be specified", i)
		}
		if item.Port != "" {
			p, err := parsePortValueV1(item.Port)
			if err != nil {
				return nil, fmt.Errorf("port: %w", err)
			}
			port = p
		}
		if item.Protocol != "" {
			tp, err := transportProtocolFromString(item.Protocol)
			if err != nil {
				return nil, err
			}
			proto = tp
		}

		out = append(out, protocol.RvTO2Addr{
			IPAddress:         ipPtr,
			DNSAddress:        dnsPtr,
			Port:              port,
			TransportProtocol: proto,
		})
	}
	return out, nil
}

func transportProtocolFromString(s string) (protocol.TransportProtocol, error) {
	switch s {
	case "tcp":
		return protocol.TCPTransport, nil
	case "tls":
		return protocol.TLSTransport, nil
	case "http":
		return protocol.HTTPTransport, nil
	case "coap":
		return protocol.CoAPTransport, nil
	case "https":
		return protocol.HTTPSTransport, nil
	case "coaps":
		return protocol.CoAPSTransport, nil
	default:
		return 0, fmt.Errorf("unsupported transport protocol %q", s)
	}
}
