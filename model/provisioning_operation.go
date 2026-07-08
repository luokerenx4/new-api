package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	ProvisioningOperationStatusStarted = "started"
	ProvisioningOperationStatusSuccess = "success"
	ProvisioningOperationStatusFailed  = "failed"
)

type ProvisioningOperation struct {
	Id                int    `json:"id"`
	OperationId       string `json:"operation_id" gorm:"type:varchar(128);uniqueIndex"`
	Action            string `json:"action" gorm:"type:varchar(64);index"`
	Status            string `json:"status" gorm:"type:varchar(32);index"`
	ExternalAccountId string `json:"external_account_id" gorm:"type:varchar(128);index"`
	UserId            int    `json:"user_id" gorm:"index"`
	TokenId           int    `json:"token_id" gorm:"index"`
	QuotaDelta        int    `json:"quota_delta" gorm:"type:int;default:0"`
	RequestPayload    string `json:"request_payload" gorm:"type:text"`
	ResponsePayload   string `json:"response_payload" gorm:"type:text"`
	ErrorMessage      string `json:"error_message" gorm:"type:text"`
	CreatedAt         int64  `json:"created_at" gorm:"autoCreateTime;column:created_at"`
}

func RecordProvisioningOperation(op *ProvisioningOperation) error {
	if op == nil {
		return errors.New("provisioning operation is nil")
	}
	if op.CreatedAt == 0 {
		op.CreatedAt = common.GetTimestamp()
	}
	return DB.Create(op).Error
}

func StartProvisioningOperation(op *ProvisioningOperation) error {
	if op == nil {
		return errors.New("provisioning operation is nil")
	}
	if op.Status == "" {
		op.Status = ProvisioningOperationStatusStarted
	}
	if op.CreatedAt == 0 {
		op.CreatedAt = common.GetTimestamp()
	}
	return DB.Create(op).Error
}

func FinishProvisioningOperation(op *ProvisioningOperation, status string, responsePayload string, errorMessage string) error {
	if op == nil || op.Id == 0 {
		return errors.New("provisioning operation is not persisted")
	}
	return DB.Model(op).Updates(map[string]interface{}{
		"status":           status,
		"response_payload": responsePayload,
		"error_message":    errorMessage,
		"user_id":          op.UserId,
		"token_id":         op.TokenId,
		"quota_delta":      op.QuotaDelta,
	}).Error
}

func GetProvisioningOperationByOperationId(operationId string) (*ProvisioningOperation, error) {
	if operationId == "" {
		return nil, errors.New("operation id is empty")
	}
	op := &ProvisioningOperation{}
	err := DB.Where("operation_id = ?", operationId).First(op).Error
	return op, err
}

func ProvisioningOperationExists(operationId string) (bool, error) {
	_, err := GetProvisioningOperationByOperationId(operationId)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, err
}
