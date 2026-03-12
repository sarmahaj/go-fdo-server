// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"

	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/fido-device-onboard/go-fdo-server/internal/manufacturing"
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
	if _, err := manufacturing.ParseOpenAPIRvJSON(data); err != nil {
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
	if _, err := manufacturing.ParseOpenAPIRvJSON(data); err != nil {
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
	return manufacturing.ParseOpenAPIRvJSON(rvInfo)
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
