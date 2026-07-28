package controller

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var openAliceUsernameUnsafeChars = regexp.MustCompile(`[^a-zA-Z0-9_]`)

const openAliceProvisioningOperationLeaseSeconds int64 = 5 * 60

type OpenAliceProvisioningUpsertUserRequest struct {
	OperationId       string `json:"operation_id"`
	ExternalAccountId string `json:"external_account_id"`
	Username          string `json:"username"`
	Email             string `json:"email"`
	DisplayName       string `json:"display_name"`
	Group             string `json:"group"`
	Status            *int   `json:"status"`
	InitialQuota      *int   `json:"initial_quota"`
}

type OpenAliceProvisioningCreateTokenRequest struct {
	OperationId        string `json:"operation_id"`
	ExternalAccountId  string `json:"external_account_id"`
	UserId             int    `json:"user_id"`
	ManagedKeyId       string `json:"managed_key_id"`
	Name               string `json:"name"`
	Quota              int    `json:"quota"`
	ExpiredTime        int64  `json:"expired_time"`
	ModelLimitsEnabled bool   `json:"model_limits_enabled"`
	ModelLimits        string `json:"model_limits"`
	AllowIps           string `json:"allow_ips"`
}

type OpenAliceProvisioningQuotaRequest struct {
	OperationId string `json:"operation_id"`
	Delta       int    `json:"delta"`
}

type OpenAliceProvisioningStatusRequest struct {
	OperationId string `json:"operation_id"`
	Status      int    `json:"status"`
}

type OpenAliceProvisioningRateLimitPolicyRequest struct {
	OperationId    string            `json:"operation_id"`
	Enabled        *bool             `json:"enabled"`
	WindowMinutes  int               `json:"window_minutes"`
	Groups         map[string][2]int `json:"groups"`
	RoutableGroups []string          `json:"routable_groups"`
}

type openAliceProvisioningRateLimitPolicyResponse struct {
	Enabled        bool              `json:"enabled"`
	WindowMinutes  int               `json:"window_minutes"`
	Groups         map[string][2]int `json:"groups"`
	RoutableGroups []string          `json:"routable_groups"`
}

type openAliceProvisioningUserResponse struct {
	Id                int     `json:"id"`
	ExternalAccountId *string `json:"external_account_id,omitempty"`
	Username          string  `json:"username"`
	Email             string  `json:"email"`
	DisplayName       string  `json:"display_name"`
	Group             string  `json:"group"`
	Status            int     `json:"status"`
	Quota             int     `json:"quota"`
	UsedQuota         int     `json:"used_quota"`
	RequestCount      int     `json:"request_count"`
}

type openAliceProvisioningTokenResponse struct {
	Id                 int    `json:"id"`
	UserId             int    `json:"user_id"`
	ManagedKeyId       string `json:"managed_key_id,omitempty"`
	Name               string `json:"name"`
	Key                string `json:"key,omitempty"`
	KeyPreview         string `json:"key_preview"`
	Status             int    `json:"status"`
	RemainQuota        int    `json:"remain_quota"`
	UsedQuota          int    `json:"used_quota"`
	ExpiredTime        int64  `json:"expired_time"`
	AccessedTime       int64  `json:"accessed_time"`
	Group              string `json:"group"`
	ModelLimitsEnabled bool   `json:"model_limits_enabled"`
	ModelLimits        string `json:"model_limits"`
}

type openAliceManagedModelPrice struct {
	Mode                     string   `json:"mode"`
	Currency                 string   `json:"currency"`
	Unit                     string   `json:"unit,omitempty"`
	InputUSDPerMillion       *float64 `json:"input_usd_per_million,omitempty"`
	OutputUSDPerMillion      *float64 `json:"output_usd_per_million,omitempty"`
	CachedInputUSDPerMillion *float64 `json:"cached_input_usd_per_million,omitempty"`
	CacheWriteUSDPerMillion  *float64 `json:"cache_write_usd_per_million,omitempty"`
	RequestUSD               *float64 `json:"request_usd,omitempty"`
	BillingExpression        string   `json:"billing_expression,omitempty"`
}

type openAliceManagedModelProtocol struct {
	Id     string `json:"id"`
	Method string `json:"method"`
	Path   string `json:"path"`
}

type openAliceManagedModel struct {
	Id                  string                          `json:"id"`
	Name                string                          `json:"name"`
	Description         string                          `json:"description,omitempty"`
	Vendor              string                          `json:"vendor,omitempty"`
	ContextWindowTokens int64                           `json:"context_window_tokens,omitempty"`
	MaxOutputTokens     int64                           `json:"max_output_tokens,omitempty"`
	Price               openAliceManagedModelPrice      `json:"price"`
	Protocols           []openAliceManagedModelProtocol `json:"protocols"`
}

func openAliceProvisioningUserPayload(user *model.User) openAliceProvisioningUserResponse {
	if user == nil {
		return openAliceProvisioningUserResponse{}
	}
	return openAliceProvisioningUserResponse{
		Id:                user.Id,
		ExternalAccountId: user.ExternalAccountId,
		Username:          user.Username,
		Email:             user.Email,
		DisplayName:       user.DisplayName,
		Group:             user.Group,
		Status:            user.Status,
		Quota:             user.Quota,
		UsedQuota:         user.UsedQuota,
		RequestCount:      user.RequestCount,
	}
}

func openAliceProvisioningTokenPayload(token *model.Token, includeKey bool) openAliceProvisioningTokenResponse {
	if token == nil {
		return openAliceProvisioningTokenResponse{}
	}
	payload := openAliceProvisioningTokenResponse{
		Id:                 token.Id,
		UserId:             token.UserId,
		Name:               token.Name,
		KeyPreview:         token.GetMaskedKey(),
		Status:             token.Status,
		RemainQuota:        token.RemainQuota,
		UsedQuota:          token.UsedQuota,
		ExpiredTime:        token.ExpiredTime,
		AccessedTime:       token.AccessedTime,
		Group:              token.Group,
		ModelLimitsEnabled: token.ModelLimitsEnabled,
		ModelLimits:        token.ModelLimits,
	}
	if token.ManagedKeyId != nil {
		payload.ManagedKeyId = *token.ManagedKeyId
	}
	if includeKey {
		payload.Key = token.GetFullKey()
	}
	return payload
}

func OpenAliceProvisioningCatalog(c *gin.Context) {
	externalAccountId := normalizeOpenAliceExternalAccountId(c.Param("external_account_id"))
	user, err := model.GetUserByExternalAccountId(externalAccountId, false)
	if err != nil {
		status := http.StatusNotFound
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusInternalServerError
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	pricings := model.GetPricing()
	models := openAliceManagedModels(pricings, model.GetVendors(), model.GetSupportedEndpointMap(), user.Group)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"models": models}})
}

func openAliceManagedModels(pricings []model.Pricing, pricingVendors []model.PricingVendor, endpointMap map[string]common.EndpointInfo, group string) []openAliceManagedModel {
	vendors := map[int]string{}
	for _, vendor := range pricingVendors {
		vendors[vendor.ID] = vendor.Name
	}
	models := make([]openAliceManagedModel, 0, len(pricings))
	for _, pricing := range pricings {
		if group != "" && !common.StringsContains(pricing.EnableGroup, group) {
			continue
		}
		price := openAliceManagedModelPrice{Currency: "USD"}
		if pricing.BillingMode == "tiered_expr" && strings.TrimSpace(pricing.BillingExpr) != "" {
			price.Mode = "expression"
			price.BillingExpression = pricing.BillingExpr
		} else if pricing.QuotaType == 1 {
			price.Mode = "per_request"
			price.Unit = "usd"
			value := pricing.ModelPrice
			price.RequestUSD = &value
		} else {
			price.Mode = "per_token"
			input := pricing.ModelRatio * 2
			output := input * pricing.CompletionRatio
			price.Unit = "usd_per_million_tokens"
			price.InputUSDPerMillion = &input
			price.OutputUSDPerMillion = &output
			if pricing.CacheRatio != nil {
				cached := input * *pricing.CacheRatio
				price.CachedInputUSDPerMillion = &cached
			}
			if pricing.CreateCacheRatio != nil {
				cacheWrite := input * *pricing.CreateCacheRatio
				price.CacheWriteUSDPerMillion = &cacheWrite
			}
		}
		protocols := make([]openAliceManagedModelProtocol, 0, len(pricing.SupportedEndpointTypes))
		for _, endpointType := range pricing.SupportedEndpointTypes {
			info, ok := endpointMap[string(endpointType)]
			if !ok || strings.TrimSpace(info.Path) == "" {
				continue
			}
			protocols = append(protocols, openAliceManagedModelProtocol{
				Id:     string(endpointType),
				Method: strings.ToUpper(common.GetStringIfEmpty(info.Method, http.MethodPost)),
				Path:   info.Path,
			})
		}
		sort.Slice(protocols, func(i, j int) bool { return protocols[i].Id < protocols[j].Id })
		models = append(models, openAliceManagedModel{
			Id:                  pricing.ModelName,
			Name:                pricing.ModelName,
			Description:         pricing.Description,
			Vendor:              vendors[pricing.VendorID],
			ContextWindowTokens: pricing.ContextLength,
			MaxOutputTokens:     pricing.MaxOutputTokens,
			Price:               price,
			Protocols:           protocols,
		})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Id < models[j].Id })
	return models
}

func openAliceProvisioningRateLimitPolicyPayload() openAliceProvisioningRateLimitPolicyResponse {
	groups := map[string][2]int{}
	raw := setting.ModelRequestRateLimitGroup2JSONString()
	_ = common.Unmarshal([]byte(raw), &groups)
	return openAliceProvisioningRateLimitPolicyResponse{
		Enabled:        setting.ModelRequestRateLimitEnabled,
		WindowMinutes:  setting.ModelRequestRateLimitDurationMinutes,
		Groups:         groups,
		RoutableGroups: setting.GetOpenAliceRoutableGroups(),
	}
}

func replayOpenAliceProvisioningOperation(c *gin.Context, op *model.ProvisioningOperation) {
	status := op.HttpStatus
	if status < 200 || status > 599 {
		status = http.StatusOK
	}
	if op.Action == "token.create" && op.TokenId > 0 {
		token, err := model.GetTokenById(op.TokenId)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		c.JSON(status, gin.H{"success": true, "data": openAliceProvisioningTokenPayload(token, true)})
		return
	}
	payload := strings.TrimSpace(op.ResponsePayload)
	if payload == "" {
		payload = `{"success":true}`
	}
	c.Data(status, "application/json; charset=utf-8", []byte(payload))
}

func resumeOpenAliceProvisioningOperation(c *gin.Context, op *model.ProvisioningOperation, action string, requestPayload string) (*model.ProvisioningOperation, bool) {
	if op.Action != action || op.RequestPayload != requestPayload {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "operation_id was already used for a different request",
			"code":    "operation_mismatch",
		})
		return nil, false
	}
	switch op.Status {
	case model.ProvisioningOperationStatusSuccess:
		if !openAliceProvisioningDesiredStateAction(action) {
			replayOpenAliceProvisioningOperation(c, op)
			return nil, false
		}
		restarted, err := model.ReapplySuccessfulProvisioningOperation(op)
		if err != nil {
			common.ApiError(c, err)
			return nil, false
		}
		if restarted {
			return op, true
		}
	case model.ProvisioningOperationStatusFailed, model.ProvisioningOperationStatusStarted:
		restarted, err := model.RestartProvisioningOperation(
			op,
			common.GetTimestamp()-openAliceProvisioningOperationLeaseSeconds,
		)
		if err != nil {
			common.ApiError(c, err)
			return nil, false
		}
		if restarted {
			return op, true
		}
	}
	c.JSON(http.StatusConflict, gin.H{
		"success": false,
		"message": "operation is already in progress",
		"code":    "operation_in_progress",
	})
	return nil, false
}

func openAliceProvisioningDesiredStateAction(action string) bool {
	return action == "user.upsert" || action == "rate_limit_policy.update"
}

func startOpenAliceProvisioningOperation(c *gin.Context, operationId string, action string, externalAccountId string, request any) (*model.ProvisioningOperation, bool) {
	operationId = strings.TrimSpace(operationId)
	if operationId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "operation_id is required"})
		return nil, false
	}
	requestPayload := "{}"
	if request != nil {
		if data, err := common.Marshal(request); err == nil {
			requestPayload = string(data)
		}
	}
	if existing, err := model.GetProvisioningOperationByOperationId(operationId); err == nil {
		return resumeOpenAliceProvisioningOperation(c, existing, action, requestPayload)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		common.ApiError(c, err)
		return nil, false
	}
	op := &model.ProvisioningOperation{
		OperationId:       operationId,
		Action:            action,
		Status:            model.ProvisioningOperationStatusStarted,
		ExternalAccountId: strings.TrimSpace(externalAccountId),
		RequestPayload:    requestPayload,
	}
	if err := model.StartProvisioningOperation(op); err != nil {
		existing, getErr := model.GetProvisioningOperationByOperationId(operationId)
		if getErr == nil {
			return resumeOpenAliceProvisioningOperation(c, existing, action, requestPayload)
		}
		common.ApiError(c, err)
		return nil, false
	}
	return op, true
}

func openAliceProvisioningResponsePayload(response any) string {
	responsePayload := "{}"
	if response != nil {
		if data, err := common.Marshal(response); err == nil {
			responsePayload = string(data)
		}
	}
	return responsePayload
}

func finishOpenAliceProvisioningOperation(op *model.ProvisioningOperation, httpStatus int, response any, errMessage string) {
	if op == nil {
		return
	}
	status := model.ProvisioningOperationStatusSuccess
	if errMessage != "" {
		status = model.ProvisioningOperationStatusFailed
	}
	responsePayload := openAliceProvisioningResponsePayload(response)
	if err := model.FinishProvisioningOperation(op, status, httpStatus, responsePayload, errMessage); err != nil {
		common.SysLog("failed to finish OpenAlice provisioning operation: " + err.Error())
	}
}

func openAliceProvisioningError(c *gin.Context, op *model.ProvisioningOperation, status int, message string) {
	finishOpenAliceProvisioningOperation(op, status, gin.H{"success": false, "message": message}, message)
	c.JSON(status, gin.H{"success": false, "message": message})
}

func normalizeOpenAliceExternalAccountId(externalAccountId string) string {
	return strings.TrimSpace(externalAccountId)
}

func defaultOpenAliceGatewayUsername(externalAccountId string) string {
	hash := common.Sha1([]byte(externalAccountId))
	return "oa_" + hash[:16]
}

func normalizeOpenAliceGatewayUsername(username string, externalAccountId string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return defaultOpenAliceGatewayUsername(externalAccountId)
	}
	username = openAliceUsernameUnsafeChars.ReplaceAllString(username, "_")
	if len(username) > model.UserNameMaxLength {
		return username[:model.UserNameMaxLength]
	}
	return username
}

func getOpenAliceProvisioningUser(userId int, externalAccountId string, selectAll bool) (*model.User, error) {
	return getOpenAliceProvisioningUserWithTx(model.DB, userId, externalAccountId, selectAll)
}

func getOpenAliceProvisioningUserWithTx(tx *gorm.DB, userId int, externalAccountId string, selectAll bool) (*model.User, error) {
	if tx == nil {
		return nil, errors.New("database transaction is nil")
	}
	user := &model.User{}
	if userId > 0 {
		query := tx
		if !selectAll {
			query = query.Omit("password")
		}
		err := query.First(user, "id = ?", userId).Error
		return user, err
	}
	externalAccountId = normalizeOpenAliceExternalAccountId(externalAccountId)
	if externalAccountId == "" {
		return nil, errors.New("external_account_id or user_id is required")
	}
	query := tx
	if !selectAll {
		query = query.Omit("password")
	}
	err := query.First(user, "external_account_id = ?", externalAccountId).Error
	return user, err
}

func adjustOpenAliceExecutionQuota(op *model.ProvisioningOperation, tokenId int, delta int) (*model.Token, error) {
	var token model.Token
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&token, "id = ?", tokenId).Error; err != nil {
			return err
		}
		if delta > 0 {
			if err := tx.Model(&model.Token{}).Where("id = ?", token.Id).
				Update("remain_quota", gorm.Expr("remain_quota + ?", delta)).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.User{}).Where("id = ?", token.UserId).
				Update("quota", gorm.Expr("quota + ?", delta)).Error; err != nil {
				return err
			}
		} else if delta < 0 {
			amount := -delta
			tokenResult := tx.Model(&model.Token{}).
				Where("id = ? AND remain_quota >= ?", token.Id, amount).
				Update("remain_quota", gorm.Expr("remain_quota - ?", amount))
			if tokenResult.Error != nil {
				return tokenResult.Error
			}
			if tokenResult.RowsAffected == 0 {
				return errors.New("token quota insufficient")
			}
			userResult := tx.Model(&model.User{}).
				Where("id = ? AND quota >= ?", token.UserId, amount).
				Update("quota", gorm.Expr("quota - ?", amount))
			if userResult.Error != nil {
				return userResult.Error
			}
			if userResult.RowsAffected == 0 {
				return errors.New("user quota insufficient")
			}
		}
		if err := tx.First(&token, "id = ?", token.Id).Error; err != nil {
			return err
		}
		op.UserId = token.UserId
		op.TokenId = token.Id
		op.QuotaDelta = delta
		response := gin.H{"success": true, "data": openAliceProvisioningTokenPayload(&token, false)}
		return model.FinishProvisioningOperationWithTx(
			tx,
			op,
			model.ProvisioningOperationStatusSuccess,
			http.StatusOK,
			openAliceProvisioningResponsePayload(response),
			"",
		)
	})
	if err != nil {
		return nil, err
	}
	if err := model.InvalidateUserCache(token.UserId); err != nil {
		common.SysLog("failed to invalidate OpenAlice user cache: " + err.Error())
	}
	if err := model.InvalidateUserTokensCache(token.UserId); err != nil {
		common.SysLog("failed to invalidate OpenAlice token cache: " + err.Error())
	}
	return &token, nil
}

func OpenAliceProvisioningUpsertUser(c *gin.Context) {
	var req OpenAliceProvisioningUpsertUserRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid request")
		return
	}
	req.ExternalAccountId = normalizeOpenAliceExternalAccountId(req.ExternalAccountId)
	op, ok := startOpenAliceProvisioningOperation(c, req.OperationId, "user.upsert", req.ExternalAccountId, req)
	if !ok {
		return
	}
	if req.ExternalAccountId == "" {
		openAliceProvisioningError(c, op, http.StatusBadRequest, "external_account_id is required")
		return
	}
	if req.InitialQuota != nil && *req.InitialQuota < 0 {
		openAliceProvisioningError(c, op, http.StatusBadRequest, "initial_quota cannot be negative")
		return
	}

	var user model.User
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		err := tx.First(&user, "external_account_id = ?", req.ExternalAccountId).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			password, genErr := common.GenerateRandomKey(32)
			if genErr != nil {
				return genErr
			}
			externalAccountId := req.ExternalAccountId
			user = model.User{
				Username:          normalizeOpenAliceGatewayUsername(req.Username, req.ExternalAccountId),
				Password:          password,
				DisplayName:       strings.TrimSpace(req.DisplayName),
				Email:             strings.TrimSpace(req.Email),
				Group:             common.GetStringIfEmpty(strings.TrimSpace(req.Group), "default"),
				Role:              common.RoleCommonUser,
				Status:            common.UserStatusEnabled,
				ExternalAccountId: &externalAccountId,
			}
			if user.DisplayName == "" {
				user.DisplayName = user.Username
			}
			if req.Status != nil {
				user.Status = *req.Status
			}
			if err := user.InsertWithTx(tx, 0); err != nil {
				return err
			}
			if req.InitialQuota != nil && *req.InitialQuota > 0 {
				if err := tx.Model(&model.User{}).Where("id = ?", user.Id).
					Update("quota", gorm.Expr("quota + ?", *req.InitialQuota)).Error; err != nil {
					return err
				}
				op.QuotaDelta = *req.InitialQuota
			}
		case err != nil:
			return err
		default:
			updates := map[string]interface{}{}
			if strings.TrimSpace(req.Email) != "" {
				updates["email"] = strings.TrimSpace(req.Email)
			}
			if strings.TrimSpace(req.DisplayName) != "" {
				updates["display_name"] = strings.TrimSpace(req.DisplayName)
			}
			if strings.TrimSpace(req.Group) != "" {
				updates["group"] = strings.TrimSpace(req.Group)
			}
			if req.Status != nil {
				updates["status"] = *req.Status
			}
			if len(updates) > 0 {
				if err := tx.Model(&model.User{}).Where("id = ?", user.Id).Updates(updates).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Model(&model.Token{}).Where("user_id = ?", user.Id).Update("group", "").Error; err != nil {
			return err
		}
		if err := tx.First(&user, "id = ?", user.Id).Error; err != nil {
			return err
		}
		op.UserId = user.Id
		response := gin.H{"success": true, "data": openAliceProvisioningUserPayload(&user)}
		return model.FinishProvisioningOperationWithTx(
			tx,
			op,
			model.ProvisioningOperationStatusSuccess,
			http.StatusOK,
			openAliceProvisioningResponsePayload(response),
			"",
		)
	})
	if err != nil {
		openAliceProvisioningError(c, op, http.StatusInternalServerError, err.Error())
		return
	}
	if err := model.InvalidateUserCache(user.Id); err != nil {
		common.SysLog("failed to invalidate OpenAlice user cache: " + err.Error())
	}
	if err := model.InvalidateUserTokensCache(user.Id); err != nil {
		common.SysLog("failed to invalidate OpenAlice token cache: " + err.Error())
	}
	response := gin.H{"success": true, "data": openAliceProvisioningUserPayload(&user)}
	c.JSON(http.StatusOK, response)
}

func OpenAliceProvisioningCreateToken(c *gin.Context) {
	var req OpenAliceProvisioningCreateTokenRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid request")
		return
	}
	req.ExternalAccountId = normalizeOpenAliceExternalAccountId(req.ExternalAccountId)
	req.ManagedKeyId = strings.TrimSpace(req.ManagedKeyId)
	op, ok := startOpenAliceProvisioningOperation(c, req.OperationId, "token.create", req.ExternalAccountId, req)
	if !ok {
		return
	}
	if req.ManagedKeyId == "" || len(req.ManagedKeyId) > 160 {
		openAliceProvisioningError(c, op, http.StatusBadRequest, "managed_key_id is required and must not exceed 160 characters")
		return
	}
	if req.Quota < 0 {
		openAliceProvisioningError(c, op, http.StatusBadRequest, "quota cannot be negative")
		return
	}

	var token model.Token
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		user, err := getOpenAliceProvisioningUserWithTx(tx, req.UserId, req.ExternalAccountId, true)
		if err != nil {
			return err
		}
		managedKeyId := req.ManagedKeyId
		err = tx.First(&token, "managed_key_id = ?", managedKeyId).Error
		switch {
		case err == nil:
			if token.UserId != user.Id {
				return errors.New("managed_key_id belongs to another user")
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			key, genErr := common.GenerateKey()
			if genErr != nil {
				return genErr
			}
			allowIps := strings.TrimSpace(req.AllowIps)
			token = model.Token{
				UserId:             user.Id,
				ManagedKeyId:       &managedKeyId,
				Name:               common.GetStringIfEmpty(strings.TrimSpace(req.Name), "OpenAlice Managed AI"),
				Key:                key,
				Status:             common.TokenStatusEnabled,
				CreatedTime:        common.GetTimestamp(),
				AccessedTime:       common.GetTimestamp(),
				ExpiredTime:        req.ExpiredTime,
				RemainQuota:        req.Quota,
				ModelLimitsEnabled: req.ModelLimitsEnabled,
				ModelLimits:        strings.TrimSpace(req.ModelLimits),
				Group:              "",
				CrossGroupRetry:    false,
			}
			if token.ExpiredTime == 0 {
				token.ExpiredTime = -1
			}
			if allowIps != "" {
				token.AllowIps = &allowIps
			}
			if err := tx.Create(&token).Error; err != nil {
				return err
			}
			if req.Quota > 0 {
				if err := tx.Model(&model.User{}).Where("id = ?", user.Id).
					Update("quota", gorm.Expr("quota + ?", req.Quota)).Error; err != nil {
					return err
				}
			}
			op.QuotaDelta = req.Quota
		default:
			return err
		}
		op.UserId = user.Id
		op.TokenId = token.Id
		auditResponse := gin.H{"success": true, "data": openAliceProvisioningTokenPayload(&token, false)}
		return model.FinishProvisioningOperationWithTx(
			tx,
			op,
			model.ProvisioningOperationStatusSuccess,
			http.StatusOK,
			openAliceProvisioningResponsePayload(auditResponse),
			"",
		)
	})
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusBadRequest
		}
		openAliceProvisioningError(c, op, status, err.Error())
		return
	}
	if err := model.InvalidateUserCache(token.UserId); err != nil {
		common.SysLog("failed to invalidate OpenAlice user cache: " + err.Error())
	}
	response := gin.H{"success": true, "data": openAliceProvisioningTokenPayload(&token, true)}
	c.JSON(http.StatusOK, response)
}

func OpenAliceProvisioningAdjustTokenQuota(c *gin.Context) {
	var req OpenAliceProvisioningQuotaRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid request")
		return
	}
	op, ok := startOpenAliceProvisioningOperation(c, req.OperationId, "token.quota_adjust", "", req)
	if !ok {
		return
	}
	token, err := adjustOpenAliceExecutionQuota(op, c.GetInt("token_id"), req.Delta)
	if err != nil {
		openAliceProvisioningError(c, op, http.StatusInternalServerError, err.Error())
		return
	}
	response := gin.H{"success": true, "data": openAliceProvisioningTokenPayload(token, false)}
	c.JSON(http.StatusOK, response)
}

func OpenAliceProvisioningUpdateTokenStatus(c *gin.Context) {
	var req OpenAliceProvisioningStatusRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid request")
		return
	}
	op, ok := startOpenAliceProvisioningOperation(c, req.OperationId, "token.status_update", "", req)
	if !ok {
		return
	}
	if req.Status != common.TokenStatusEnabled && req.Status != common.TokenStatusDisabled && req.Status != common.TokenStatusExpired && req.Status != common.TokenStatusExhausted {
		openAliceProvisioningError(c, op, http.StatusBadRequest, "invalid token status")
		return
	}
	var token model.Token
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&token, "id = ?", c.GetInt("token_id")).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Token{}).Where("id = ?", token.Id).Update("status", req.Status).Error; err != nil {
			return err
		}
		token.Status = req.Status
		op.UserId = token.UserId
		op.TokenId = token.Id
		response := gin.H{"success": true, "data": openAliceProvisioningTokenPayload(&token, false)}
		return model.FinishProvisioningOperationWithTx(
			tx,
			op,
			model.ProvisioningOperationStatusSuccess,
			http.StatusOK,
			openAliceProvisioningResponsePayload(response),
			"",
		)
	})
	if err != nil {
		openAliceProvisioningError(c, op, http.StatusInternalServerError, err.Error())
		return
	}
	if err := model.InvalidateUserTokensCache(token.UserId); err != nil {
		common.SysLog("failed to invalidate OpenAlice token cache: " + err.Error())
	}
	response := gin.H{"success": true, "data": openAliceProvisioningTokenPayload(&token, false)}
	c.JSON(http.StatusOK, response)
}

func OpenAliceProvisioningUpdateRateLimitPolicy(c *gin.Context) {
	var req OpenAliceProvisioningRateLimitPolicyRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid request")
		return
	}
	op, ok := startOpenAliceProvisioningOperation(c, req.OperationId, "rate_limit_policy.update", "", req)
	if !ok {
		return
	}
	if req.WindowMinutes < 1 || req.WindowMinutes > 1440 {
		openAliceProvisioningError(c, op, http.StatusBadRequest, "window_minutes must be between 1 and 1440")
		return
	}
	if len(req.Groups) == 0 {
		openAliceProvisioningError(c, op, http.StatusBadRequest, "groups is required")
		return
	}
	if req.RoutableGroups == nil {
		openAliceProvisioningError(c, op, http.StatusBadRequest, "routable_groups is required")
		return
	}
	for group := range req.Groups {
		if group != strings.TrimSpace(group) || !validOpenAliceProvisioningGroup(group) {
			openAliceProvisioningError(c, op, http.StatusBadRequest, "invalid group name")
			return
		}
	}
	groupsJSONBytes, err := common.Marshal(req.Groups)
	if err != nil {
		openAliceProvisioningError(c, op, http.StatusBadRequest, err.Error())
		return
	}
	groupsJSON := string(groupsJSONBytes)
	if err := setting.CheckModelRequestRateLimitGroup(groupsJSON); err != nil {
		openAliceProvisioningError(c, op, http.StatusBadRequest, err.Error())
		return
	}
	groupNames := openAliceProvisioningSortedGroupNames(req.Groups)
	routableGroups := openAliceProvisioningSortedStringSet(req.RoutableGroups)
	groupSet := make(map[string]bool, len(groupNames))
	for _, group := range groupNames {
		groupSet[group] = true
	}
	for _, group := range routableGroups {
		if !validOpenAliceProvisioningGroup(group) || !groupSet[group] {
			openAliceProvisioningError(c, op, http.StatusBadRequest, "routable_groups must be a subset of groups")
			return
		}
	}
	previousManagedGroups := setting.GetOpenAliceManagedGroups()
	usableGroupsJSON, err := openAliceProvisioningDesiredUsableGroupsJSON(previousManagedGroups, groupNames)
	if err != nil {
		openAliceProvisioningError(c, op, http.StatusInternalServerError, err.Error())
		return
	}
	groupRatioJSON, err := openAliceProvisioningDesiredGroupRatioJSON(previousManagedGroups, groupNames)
	if err != nil {
		openAliceProvisioningError(c, op, http.StatusInternalServerError, err.Error())
		return
	}
	managedGroupsJSONBytes, err := common.Marshal(groupNames)
	if err != nil {
		openAliceProvisioningError(c, op, http.StatusInternalServerError, err.Error())
		return
	}
	routableGroupsJSONBytes, err := common.Marshal(routableGroups)
	if err != nil {
		openAliceProvisioningError(c, op, http.StatusInternalServerError, err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if err := openAliceProvisioningSyncManagedChannelGroups(previousManagedGroups, routableGroups); err != nil {
		openAliceProvisioningError(c, op, http.StatusInternalServerError, err.Error())
		return
	}
	if err := model.UpdateOptionsBulk(map[string]string{
		"ModelRequestRateLimitEnabled":         strconv.FormatBool(enabled),
		"ModelRequestRateLimitDurationMinutes": strconv.Itoa(req.WindowMinutes),
		"ModelRequestRateLimitGroup":           groupsJSON,
		"UserUsableGroups":                     usableGroupsJSON,
		"GroupRatio":                           groupRatioJSON,
		"OpenAliceManagedGroups":               string(managedGroupsJSONBytes),
		"OpenAliceRoutableGroups":              string(routableGroupsJSONBytes),
	}); err != nil {
		openAliceProvisioningError(c, op, http.StatusInternalServerError, err.Error())
		return
	}
	response := gin.H{"success": true, "data": openAliceProvisioningRateLimitPolicyPayload()}
	finishOpenAliceProvisioningOperation(op, http.StatusOK, response, "")
	c.JSON(http.StatusOK, response)
}

func OpenAliceProvisioningSnapshot(c *gin.Context) {
	externalAccountId := normalizeOpenAliceExternalAccountId(c.Param("external_account_id"))
	user, err := model.GetUserByExternalAccountId(externalAccountId, true)
	if err != nil {
		status := http.StatusNotFound
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusInternalServerError
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	tokens, err := model.GetAllUserTokens(user.Id, 0, 1000)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tokenPayloads := make([]openAliceProvisioningTokenResponse, 0, len(tokens))
	for _, token := range tokens {
		tokenPayloads = append(tokenPayloads, openAliceProvisioningTokenPayload(token, false))
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"user":   openAliceProvisioningUserPayload(user),
			"tokens": tokenPayloads,
		},
	})
}

func validOpenAliceProvisioningGroup(group string) bool {
	group = strings.TrimSpace(group)
	if group == "" || len(group) > 64 {
		return false
	}
	for _, r := range group {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func openAliceProvisioningSortedGroupNames(groups map[string][2]int) []string {
	groupNames := make([]string, 0, len(groups))
	for group := range groups {
		group = strings.TrimSpace(group)
		if group != "" {
			groupNames = append(groupNames, group)
		}
	}
	sort.Strings(groupNames)
	return groupNames
}

func openAliceProvisioningSortedStringSet(groups []string) []string {
	seen := make(map[string]bool, len(groups))
	out := make([]string, 0, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" || seen[group] {
			continue
		}
		seen[group] = true
		out = append(out, group)
	}
	sort.Strings(out)
	return out
}

func openAliceProvisioningDesiredUsableGroupsJSON(previousManagedGroups []string, groupNames []string) (string, error) {
	usableGroups := map[string]string{}
	raw := strings.TrimSpace(setting.UserUsableGroups2JSONString())
	if raw != "" {
		if err := common.Unmarshal([]byte(raw), &usableGroups); err != nil {
			return "", err
		}
	}
	for _, group := range previousManagedGroups {
		delete(usableGroups, group)
	}
	for _, group := range groupNames {
		usableGroups[group] = "OpenAlice managed " + group
	}
	data, err := common.Marshal(usableGroups)
	return string(data), err
}

func openAliceProvisioningDesiredGroupRatioJSON(previousManagedGroups []string, groupNames []string) (string, error) {
	groupRatios := ratio_setting.GetGroupRatioCopy()
	for _, group := range previousManagedGroups {
		delete(groupRatios, group)
	}
	for _, group := range groupNames {
		groupRatios[group] = 1
	}
	data, err := common.Marshal(groupRatios)
	return string(data), err
}

func openAliceProvisioningSyncManagedChannelGroups(previousManagedGroups []string, routableGroups []string) error {
	if !common.OpenAliceManagedMode {
		return nil
	}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var channels []model.Channel
		if err := tx.Find(&channels).Error; err != nil {
			return err
		}
		for _, channel := range channels {
			merged := openAliceProvisioningReplaceGroupCSV(channel.Group, previousManagedGroups, routableGroups)
			channel.Group = merged
			if err := tx.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("group", merged).Error; err != nil {
				return err
			}
			if err := channel.UpdateAbilities(tx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	model.InitChannelCache()
	return nil
}

func openAliceProvisioningReplaceGroupCSV(existing string, removeGroups []string, addGroups []string) string {
	remove := make(map[string]bool, len(removeGroups))
	for _, group := range removeGroups {
		remove[strings.TrimSpace(group)] = true
	}
	seen := map[string]bool{}
	merged := make([]string, 0, len(addGroups)+1)
	for _, group := range strings.Split(existing, ",") {
		group = strings.TrimSpace(group)
		if group == "" || seen[group] || remove[group] {
			continue
		}
		seen[group] = true
		merged = append(merged, group)
	}
	for _, group := range addGroups {
		group = strings.TrimSpace(group)
		if group == "" || seen[group] {
			continue
		}
		seen[group] = true
		merged = append(merged, group)
	}
	return strings.Join(merged, ",")
}

func OpenAliceProvisioningTokenIdParam(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": fmt.Sprintf("invalid token id: %s", c.Param("id"))})
		c.Abort()
		return
	}
	c.Set("token_id", id)
	c.Next()
}
