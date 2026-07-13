package controller

import (
	"encoding/json"
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
	Name               string `json:"name"`
	Quota              int    `json:"quota"`
	ExpiredTime        int64  `json:"expired_time"`
	Group              string `json:"group"`
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
	OperationId   string            `json:"operation_id"`
	Enabled       *bool             `json:"enabled"`
	WindowMinutes int               `json:"window_minutes"`
	Groups        map[string][2]int `json:"groups"`
}

type openAliceProvisioningRateLimitPolicyResponse struct {
	Enabled       bool              `json:"enabled"`
	WindowMinutes int               `json:"window_minutes"`
	Groups        map[string][2]int `json:"groups"`
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
	Unit                     string   `json:"unit"`
	InputUSDPerMillion       *float64 `json:"input_usd_per_million,omitempty"`
	OutputUSDPerMillion      *float64 `json:"output_usd_per_million,omitempty"`
	CachedInputUSDPerMillion *float64 `json:"cached_input_usd_per_million,omitempty"`
	CacheWriteUSDPerMillion  *float64 `json:"cache_write_usd_per_million,omitempty"`
	RequestUSD               *float64 `json:"request_usd,omitempty"`
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
	if includeKey {
		payload.Key = token.GetFullKey()
	}
	return payload
}

func OpenAliceProvisioningCatalog(c *gin.Context) {
	pricings := model.GetPricing()
	models := openAliceManagedModels(pricings, model.GetVendors(), model.GetSupportedEndpointMap())
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"models": models}})
}

func openAliceManagedModels(pricings []model.Pricing, pricingVendors []model.PricingVendor, endpointMap map[string]common.EndpointInfo) []openAliceManagedModel {
	vendors := map[int]string{}
	for _, vendor := range pricingVendors {
		vendors[vendor.ID] = vendor.Name
	}
	models := make([]openAliceManagedModel, 0, len(pricings))
	for _, pricing := range pricings {
		price := openAliceManagedModelPrice{Unit: "usd"}
		if pricing.QuotaType == 1 {
			value := pricing.ModelPrice
			price.RequestUSD = &value
		} else {
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
	_ = json.Unmarshal([]byte(raw), &groups)
	return openAliceProvisioningRateLimitPolicyResponse{
		Enabled:       setting.ModelRequestRateLimitEnabled,
		WindowMinutes: setting.ModelRequestRateLimitDurationMinutes,
		Groups:        groups,
	}
}

func startOpenAliceProvisioningOperation(c *gin.Context, operationId string, action string, externalAccountId string, request any) (*model.ProvisioningOperation, bool) {
	operationId = strings.TrimSpace(operationId)
	if operationId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "operation_id is required"})
		return nil, false
	}
	if exists, err := model.ProvisioningOperationExists(operationId); err != nil {
		common.ApiError(c, err)
		return nil, false
	} else if exists {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "duplicate operation_id", "code": "duplicate_operation"})
		return nil, false
	}
	requestPayload := "{}"
	if request != nil {
		if data, err := common.Marshal(request); err == nil {
			requestPayload = string(data)
		}
	}
	op := &model.ProvisioningOperation{
		OperationId:       operationId,
		Action:            action,
		Status:            model.ProvisioningOperationStatusStarted,
		ExternalAccountId: strings.TrimSpace(externalAccountId),
		RequestPayload:    requestPayload,
	}
	if err := model.StartProvisioningOperation(op); err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	return op, true
}

func finishOpenAliceProvisioningOperation(op *model.ProvisioningOperation, response any, errMessage string) {
	if op == nil {
		return
	}
	status := model.ProvisioningOperationStatusSuccess
	if errMessage != "" {
		status = model.ProvisioningOperationStatusFailed
	}
	responsePayload := "{}"
	if response != nil {
		if data, err := common.Marshal(response); err == nil {
			responsePayload = string(data)
		}
	}
	if err := model.FinishProvisioningOperation(op, status, responsePayload, errMessage); err != nil {
		common.SysLog("failed to finish OpenAlice provisioning operation: " + err.Error())
	}
}

func openAliceProvisioningError(c *gin.Context, op *model.ProvisioningOperation, status int, message string) {
	finishOpenAliceProvisioningOperation(op, gin.H{"success": false, "message": message}, message)
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
	if userId > 0 {
		return model.GetUserById(userId, selectAll)
	}
	externalAccountId = normalizeOpenAliceExternalAccountId(externalAccountId)
	if externalAccountId == "" {
		return nil, errors.New("external_account_id or user_id is required")
	}
	return model.GetUserByExternalAccountId(externalAccountId, selectAll)
}

func adjustOpenAliceExecutionQuota(userId int, token *model.Token, delta int) error {
	if delta == 0 {
		return nil
	}
	if token == nil {
		return errors.New("token is nil")
	}
	if delta > 0 {
		ok, err := model.AdjustTokenRemainQuota(token.Id, token.Key, delta)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("token quota adjustment rejected")
		}
		if err := model.IncreaseUserQuota(userId, delta, true); err != nil {
			_, _ = model.AdjustTokenRemainQuota(token.Id, token.Key, -delta)
			return err
		}
		return nil
	}

	amount := -delta
	ok, err := model.TryPreConsumeUserQuota(userId, amount)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("user quota insufficient")
	}
	ok, err = model.AdjustTokenRemainQuota(token.Id, token.Key, delta)
	if err != nil {
		_ = model.IncreaseUserQuota(userId, amount, true)
		return err
	}
	if !ok {
		_ = model.IncreaseUserQuota(userId, amount, true)
		return errors.New("token quota insufficient")
	}
	return nil
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

	user, err := model.GetUserByExternalAccountId(req.ExternalAccountId, true)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		openAliceProvisioningError(c, op, http.StatusInternalServerError, err.Error())
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		password, genErr := common.GenerateRandomKey(32)
		if genErr != nil {
			openAliceProvisioningError(c, op, http.StatusInternalServerError, genErr.Error())
			return
		}
		externalAccountId := req.ExternalAccountId
		user = &model.User{
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
		if err := user.Insert(0); err != nil {
			openAliceProvisioningError(c, op, http.StatusBadRequest, err.Error())
			return
		}
		if req.InitialQuota != nil && *req.InitialQuota > 0 {
			if err := model.IncreaseUserQuota(user.Id, *req.InitialQuota, true); err != nil {
				openAliceProvisioningError(c, op, http.StatusInternalServerError, err.Error())
				return
			}
			user.Quota += *req.InitialQuota
			op.QuotaDelta = *req.InitialQuota
		}
	} else {
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
			if err := model.DB.Model(user).Updates(updates).Error; err != nil {
				openAliceProvisioningError(c, op, http.StatusInternalServerError, err.Error())
				return
			}
			user, _ = model.GetUserById(user.Id, true)
		}
	}

	op.UserId = user.Id
	response := gin.H{"success": true, "data": openAliceProvisioningUserPayload(user)}
	finishOpenAliceProvisioningOperation(op, response, "")
	c.JSON(http.StatusOK, response)
}

func OpenAliceProvisioningCreateToken(c *gin.Context) {
	var req OpenAliceProvisioningCreateTokenRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid request")
		return
	}
	req.ExternalAccountId = normalizeOpenAliceExternalAccountId(req.ExternalAccountId)
	op, ok := startOpenAliceProvisioningOperation(c, req.OperationId, "token.create", req.ExternalAccountId, req)
	if !ok {
		return
	}
	user, err := getOpenAliceProvisioningUser(req.UserId, req.ExternalAccountId, true)
	if err != nil {
		status := http.StatusBadRequest
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusInternalServerError
		}
		openAliceProvisioningError(c, op, status, err.Error())
		return
	}
	if req.Quota < 0 {
		openAliceProvisioningError(c, op, http.StatusBadRequest, "quota cannot be negative")
		return
	}
	key, err := common.GenerateKey()
	if err != nil {
		openAliceProvisioningError(c, op, http.StatusInternalServerError, err.Error())
		return
	}
	allowIps := strings.TrimSpace(req.AllowIps)
	token := &model.Token{
		UserId:             user.Id,
		Name:               common.GetStringIfEmpty(strings.TrimSpace(req.Name), "OpenAlice Managed AI"),
		Key:                key,
		Status:             common.TokenStatusEnabled,
		CreatedTime:        common.GetTimestamp(),
		AccessedTime:       common.GetTimestamp(),
		ExpiredTime:        req.ExpiredTime,
		RemainQuota:        req.Quota,
		ModelLimitsEnabled: req.ModelLimitsEnabled,
		ModelLimits:        strings.TrimSpace(req.ModelLimits),
		Group:              common.GetStringIfEmpty(strings.TrimSpace(req.Group), user.Group),
		CrossGroupRetry:    false,
	}
	if token.ExpiredTime == 0 {
		token.ExpiredTime = -1
	}
	if allowIps != "" {
		token.AllowIps = &allowIps
	}
	if err := token.Insert(); err != nil {
		openAliceProvisioningError(c, op, http.StatusInternalServerError, err.Error())
		return
	}
	if req.Quota > 0 {
		if err := model.IncreaseUserQuota(user.Id, req.Quota, true); err != nil {
			_ = token.Delete()
			openAliceProvisioningError(c, op, http.StatusInternalServerError, err.Error())
			return
		}
	}
	op.UserId = user.Id
	op.TokenId = token.Id
	op.QuotaDelta = req.Quota

	response := gin.H{"success": true, "data": openAliceProvisioningTokenPayload(token, true)}
	auditResponse := gin.H{"success": true, "data": openAliceProvisioningTokenPayload(token, false)}
	finishOpenAliceProvisioningOperation(op, auditResponse, "")
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
	token, err := model.GetTokenById(c.GetInt("token_id"))
	if err != nil {
		openAliceProvisioningError(c, op, http.StatusBadRequest, err.Error())
		return
	}
	if err := adjustOpenAliceExecutionQuota(token.UserId, token, req.Delta); err != nil {
		openAliceProvisioningError(c, op, http.StatusInternalServerError, err.Error())
		return
	}
	token, _ = model.GetTokenById(token.Id)
	op.UserId = token.UserId
	op.TokenId = token.Id
	op.QuotaDelta = req.Delta
	response := gin.H{"success": true, "data": openAliceProvisioningTokenPayload(token, false)}
	finishOpenAliceProvisioningOperation(op, response, "")
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
	token, err := model.GetTokenById(c.GetInt("token_id"))
	if err != nil {
		openAliceProvisioningError(c, op, http.StatusBadRequest, err.Error())
		return
	}
	token.Status = req.Status
	if err := token.Update(); err != nil {
		openAliceProvisioningError(c, op, http.StatusInternalServerError, err.Error())
		return
	}
	op.UserId = token.UserId
	op.TokenId = token.Id
	response := gin.H{"success": true, "data": openAliceProvisioningTokenPayload(token, false)}
	finishOpenAliceProvisioningOperation(op, response, "")
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
	for group := range req.Groups {
		if !validOpenAliceProvisioningGroup(group) {
			openAliceProvisioningError(c, op, http.StatusBadRequest, "invalid group name")
			return
		}
	}
	groupsJSONBytes, err := json.Marshal(req.Groups)
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
	usableGroupsJSON, err := openAliceProvisioningMergedUsableGroupsJSON(groupNames)
	if err != nil {
		openAliceProvisioningError(c, op, http.StatusInternalServerError, err.Error())
		return
	}
	groupRatioJSON, err := openAliceProvisioningMergedGroupRatioJSON(groupNames)
	if err != nil {
		openAliceProvisioningError(c, op, http.StatusInternalServerError, err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if err := model.UpdateOptionsBulk(map[string]string{
		"ModelRequestRateLimitEnabled":         strconv.FormatBool(enabled),
		"ModelRequestRateLimitDurationMinutes": strconv.Itoa(req.WindowMinutes),
		"ModelRequestRateLimitGroup":           groupsJSON,
		"UserUsableGroups":                     usableGroupsJSON,
		"GroupRatio":                           groupRatioJSON,
	}); err != nil {
		openAliceProvisioningError(c, op, http.StatusInternalServerError, err.Error())
		return
	}
	if err := openAliceProvisioningSyncManagedChannelGroups(groupNames); err != nil {
		openAliceProvisioningError(c, op, http.StatusInternalServerError, err.Error())
		return
	}
	response := gin.H{"success": true, "data": openAliceProvisioningRateLimitPolicyPayload()}
	finishOpenAliceProvisioningOperation(op, response, "")
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

func openAliceProvisioningMergedUsableGroupsJSON(groupNames []string) (string, error) {
	usableGroups := map[string]string{}
	raw := strings.TrimSpace(setting.UserUsableGroups2JSONString())
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &usableGroups); err != nil {
			return "", err
		}
	}
	for _, group := range groupNames {
		if _, ok := usableGroups[group]; !ok {
			usableGroups[group] = "OpenAlice managed " + group
		}
	}
	data, err := json.Marshal(usableGroups)
	return string(data), err
}

func openAliceProvisioningMergedGroupRatioJSON(groupNames []string) (string, error) {
	groupRatios := ratio_setting.GetGroupRatioCopy()
	for _, group := range groupNames {
		if _, ok := groupRatios[group]; !ok {
			groupRatios[group] = 1
		}
	}
	data, err := json.Marshal(groupRatios)
	return string(data), err
}

func openAliceProvisioningSyncManagedChannelGroups(groupNames []string) error {
	if !common.OpenAliceManagedMode || len(groupNames) == 0 {
		return nil
	}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var channels []model.Channel
		if err := tx.Find(&channels).Error; err != nil {
			return err
		}
		for _, channel := range channels {
			merged := openAliceProvisioningMergeGroupCSV(channel.Group, groupNames)
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

func openAliceProvisioningMergeGroupCSV(existing string, groupNames []string) string {
	seen := map[string]bool{}
	merged := make([]string, 0, len(groupNames)+1)
	for _, group := range strings.Split(existing, ",") {
		group = strings.TrimSpace(group)
		if group == "" || seen[group] {
			continue
		}
		seen[group] = true
		merged = append(merged, group)
	}
	for _, group := range groupNames {
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
