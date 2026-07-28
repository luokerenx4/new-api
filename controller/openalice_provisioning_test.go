package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAliceProvisioningAPIResponse struct {
	Success bool                           `json:"success"`
	Data    openAliceProvisioningTokenData `json:"data"`
}

type openAliceProvisioningTokenData struct {
	Id           int    `json:"id"`
	UserId       int    `json:"user_id"`
	ManagedKeyId string `json:"managed_key_id"`
	Key          string `json:"key"`
	KeyPreview   string `json:"key_preview"`
	RemainQuota  int    `json:"remain_quota"`
}

func setupOpenAliceProvisioningControllerTestDB(t *testing.T) {
	t.Helper()
	db := openTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.Option{}, &model.ProvisioningOperation{}))

	originalQuotaForNewUser := common.QuotaForNewUser
	originalBatchUpdateEnabled := common.BatchUpdateEnabled
	originalManagedMode := common.OpenAliceManagedMode
	originalRateLimitEnabled := setting.ModelRequestRateLimitEnabled
	originalRateLimitDuration := setting.ModelRequestRateLimitDurationMinutes
	originalRateLimitCount := setting.ModelRequestRateLimitCount
	originalRateLimitSuccessCount := setting.ModelRequestRateLimitSuccessCount
	originalRateLimitGroup := setting.ModelRequestRateLimitGroup2JSONString()
	originalUserUsableGroups := setting.UserUsableGroups2JSONString()
	originalGroupRatio := ratio_setting.GroupRatio2JSONString()
	originalManagedGroups := setting.OpenAliceManagedGroups2JSONString()
	originalRoutableGroups := setting.OpenAliceRoutableGroups2JSONString()
	t.Cleanup(func() {
		common.QuotaForNewUser = originalQuotaForNewUser
		common.BatchUpdateEnabled = originalBatchUpdateEnabled
		common.OpenAliceManagedMode = originalManagedMode
		setting.ModelRequestRateLimitEnabled = originalRateLimitEnabled
		setting.ModelRequestRateLimitDurationMinutes = originalRateLimitDuration
		setting.ModelRequestRateLimitCount = originalRateLimitCount
		setting.ModelRequestRateLimitSuccessCount = originalRateLimitSuccessCount
		_ = setting.UpdateModelRequestRateLimitGroupByJSONString(originalRateLimitGroup)
		_ = setting.UpdateUserUsableGroupsByJSONString(originalUserUsableGroups)
		_ = ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio)
		_ = setting.UpdateOpenAliceManagedGroupsByJSONString(originalManagedGroups)
		_ = setting.UpdateOpenAliceRoutableGroupsByJSONString(originalRoutableGroups)
		model.InitOptionMap()
	})
	common.QuotaForNewUser = 0
	common.BatchUpdateEnabled = false
	common.OpenAliceManagedMode = false
	setting.ModelRequestRateLimitEnabled = false
	setting.ModelRequestRateLimitDurationMinutes = 1
	setting.ModelRequestRateLimitCount = 0
	setting.ModelRequestRateLimitSuccessCount = 1000
	_ = setting.UpdateModelRequestRateLimitGroupByJSONString(`{}`)
	_ = setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`)
	_ = ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`)
	_ = setting.UpdateOpenAliceManagedGroupsByJSONString(`[]`)
	_ = setting.UpdateOpenAliceRoutableGroupsByJSONString(`[]`)
	model.InitOptionMap()
}

func newOpenAliceProvisioningContext(t *testing.T, body any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	payload, err := common.Marshal(body)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/openalice/provisioning/test", bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx, recorder
}

func TestOpenAliceProvisioningCreatesUserTokenAndAuditsWithoutRawKey(t *testing.T) {
	setupOpenAliceProvisioningControllerTestDB(t)

	upsertReq := OpenAliceProvisioningUpsertUserRequest{
		OperationId:       "op-user-create",
		ExternalAccountId: "acct_test_1",
		Email:             "acct@example.com",
		InitialQuota:      common.GetPointer(25),
	}
	ctx, recorder := newOpenAliceProvisioningContext(t, upsertReq)
	OpenAliceProvisioningUpsertUser(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	user, err := model.GetUserByExternalAccountId("acct_test_1", true)
	require.NoError(t, err)
	require.Equal(t, 25, user.Quota)

	ctx, recorder = newOpenAliceProvisioningContext(t, upsertReq)
	OpenAliceProvisioningUpsertUser(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	user, err = model.GetUserByExternalAccountId("acct_test_1", true)
	require.NoError(t, err)
	require.Equal(t, 25, user.Quota)
	userOperation, err := model.GetProvisioningOperationByOperationId(upsertReq.OperationId)
	require.NoError(t, err)
	require.Equal(t, 2, userOperation.Attempts)

	tokenReq := OpenAliceProvisioningCreateTokenRequest{
		OperationId:       "op-token-create",
		ExternalAccountId: "acct_test_1",
		ManagedKeyId:      "acct_test_1:desktop:1",
		Name:              "OpenAlice Managed AI",
		Quota:             100,
	}
	ctx, recorder = newOpenAliceProvisioningContext(t, tokenReq)
	OpenAliceProvisioningCreateToken(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var response openAliceProvisioningAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.NotEmpty(t, response.Data.Key)
	require.NotEmpty(t, response.Data.KeyPreview)
	require.Equal(t, tokenReq.ManagedKeyId, response.Data.ManagedKeyId)
	require.Equal(t, 100, response.Data.RemainQuota)
	firstKey := response.Data.Key

	user, err = model.GetUserByExternalAccountId("acct_test_1", true)
	require.NoError(t, err)
	require.Equal(t, 125, user.Quota)

	var token model.Token
	require.NoError(t, model.DB.First(&token, response.Data.Id).Error)
	require.Equal(t, 100, token.RemainQuota)
	require.Equal(t, 0, token.UsedQuota)

	op, err := model.GetProvisioningOperationByOperationId("op-token-create")
	require.NoError(t, err)
	require.Equal(t, model.ProvisioningOperationStatusSuccess, op.Status)
	require.Contains(t, op.ResponsePayload, "key_preview")
	require.False(t, strings.Contains(op.ResponsePayload, response.Data.Key))

	ctx, recorder = newOpenAliceProvisioningContext(t, tokenReq)
	OpenAliceProvisioningCreateToken(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, firstKey, response.Data.Key)

	var tokenCount int64
	require.NoError(t, model.DB.Model(&model.Token{}).Where("user_id = ?", user.Id).Count(&tokenCount).Error)
	require.EqualValues(t, 1, tokenCount)
	user, err = model.GetUserByExternalAccountId("acct_test_1", true)
	require.NoError(t, err)
	require.Equal(t, 125, user.Quota)
	op, err = model.GetProvisioningOperationByOperationId("op-token-create")
	require.NoError(t, err)
	require.Equal(t, 1, op.Attempts)
}

func TestOpenAliceProvisioningTokenPayloadIncludesLastAccessTime(t *testing.T) {
	payload := openAliceProvisioningTokenPayload(&model.Token{Id: 7, AccessedTime: 123456789}, false)
	require.Equal(t, int64(123456789), payload.AccessedTime)
}

func TestOpenAliceManagedModelsProjectsPricingProtocolsAndMetadata(t *testing.T) {
	cacheRatio := 0.02
	models := openAliceManagedModels(
		[]model.Pricing{{
			ModelName: "LongCat-2.0", Description: "Agentic model", VendorID: 9,
			ContextLength: 1048576, MaxOutputTokens: 131072,
			QuotaType: 0, ModelRatio: 0.375, CompletionRatio: 3.9333333333, CacheRatio: &cacheRatio,
			SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAI},
			EnableGroup:            []string{"starter", "pro"},
		}},
		[]model.PricingVendor{{ID: 9, Name: "Meituan LongCat"}},
		map[string]common.EndpointInfo{"openai": {Method: http.MethodPost, Path: "/v1/chat/completions"}},
		"starter",
	)
	require.Len(t, models, 1)
	managed := models[0]
	require.Equal(t, "LongCat-2.0", managed.Id)
	require.Equal(t, "Meituan LongCat", managed.Vendor)
	require.Equal(t, int64(1048576), managed.ContextWindowTokens)
	require.NotNil(t, managed.Price.InputUSDPerMillion)
	require.InDelta(t, 0.75, *managed.Price.InputUSDPerMillion, 0.000001)
	require.NotNil(t, managed.Price.OutputUSDPerMillion)
	require.InDelta(t, 2.95, *managed.Price.OutputUSDPerMillion, 0.000001)
	require.NotNil(t, managed.Price.CachedInputUSDPerMillion)
	require.InDelta(t, 0.015, *managed.Price.CachedInputUSDPerMillion, 0.000001)
	require.Equal(t, []openAliceManagedModelProtocol{{Id: "openai", Method: http.MethodPost, Path: "/v1/chat/completions"}}, managed.Protocols)
}

func TestOpenAliceManagedModelsFiltersByAccountGroupAndPreservesDynamicPricing(t *testing.T) {
	models := openAliceManagedModels(
		[]model.Pricing{
			{
				ModelName:   "starter-only",
				QuotaType:   1,
				ModelPrice:  0.02,
				EnableGroup: []string{"starter"},
			},
			{
				ModelName:     "claude-dynamic",
				BillingMode:   "tiered_expr",
				BillingExpr:   `len <= 200000 ? tier("standard", p * 3 + c * 15) : tier("long", p * 6 + c * 22.5)`,
				EnableGroup:   []string{"pro"},
				ContextLength: 1000000,
			},
		},
		nil,
		nil,
		"pro",
	)

	require.Len(t, models, 1)
	require.Equal(t, "claude-dynamic", models[0].Id)
	require.Equal(t, "expression", models[0].Price.Mode)
	require.Equal(t, "USD", models[0].Price.Currency)
	require.Contains(t, models[0].Price.BillingExpression, `tier("long"`)
	require.Nil(t, models[0].Price.InputUSDPerMillion)
	require.Nil(t, models[0].Price.RequestUSD)
}

func TestOpenAliceProvisioningReappliesSuccessfulDesiredStateOperation(t *testing.T) {
	setupOpenAliceProvisioningControllerTestDB(t)

	req := OpenAliceProvisioningUpsertUserRequest{
		OperationId:       "op-duplicate",
		ExternalAccountId: "acct_dup",
		Group:             "pro",
	}
	ctx, recorder := newOpenAliceProvisioningContext(t, req)
	OpenAliceProvisioningUpsertUser(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	user, err := model.GetUserByExternalAccountId(req.ExternalAccountId, true)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("group", "legacy").Error)

	ctx, recorder = newOpenAliceProvisioningContext(t, req)
	OpenAliceProvisioningUpsertUser(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"external_account_id":"acct_dup"`)

	user, err = model.GetUserByExternalAccountId(req.ExternalAccountId, true)
	require.NoError(t, err)
	require.Equal(t, "pro", user.Group)
	op, err := model.GetProvisioningOperationByOperationId(req.OperationId)
	require.NoError(t, err)
	require.Equal(t, 2, op.Attempts)
}

func TestOpenAliceProvisioningRetriesFailedOperation(t *testing.T) {
	setupOpenAliceProvisioningControllerTestDB(t)

	tokenReq := OpenAliceProvisioningCreateTokenRequest{
		OperationId:       "op-token-retry",
		ExternalAccountId: "acct_retry",
		ManagedKeyId:      "acct_retry:desktop:1",
		Quota:             50,
	}
	ctx, recorder := newOpenAliceProvisioningContext(t, tokenReq)
	OpenAliceProvisioningCreateToken(ctx)
	require.Equal(t, http.StatusBadRequest, recorder.Code)

	op, err := model.GetProvisioningOperationByOperationId(tokenReq.OperationId)
	require.NoError(t, err)
	require.Equal(t, model.ProvisioningOperationStatusFailed, op.Status)
	require.Equal(t, 1, op.Attempts)

	upsertReq := OpenAliceProvisioningUpsertUserRequest{
		OperationId:       "op-user-for-token-retry",
		ExternalAccountId: tokenReq.ExternalAccountId,
	}
	ctx, recorder = newOpenAliceProvisioningContext(t, upsertReq)
	OpenAliceProvisioningUpsertUser(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	ctx, recorder = newOpenAliceProvisioningContext(t, tokenReq)
	OpenAliceProvisioningCreateToken(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	op, err = model.GetProvisioningOperationByOperationId(tokenReq.OperationId)
	require.NoError(t, err)
	require.Equal(t, model.ProvisioningOperationStatusSuccess, op.Status)
	require.Equal(t, 2, op.Attempts)
	user, err := model.GetUserByExternalAccountId(tokenReq.ExternalAccountId, true)
	require.NoError(t, err)
	require.Equal(t, 50, user.Quota)
}

func TestOpenAliceProvisioningRejectsOperationPayloadMismatch(t *testing.T) {
	setupOpenAliceProvisioningControllerTestDB(t)

	req := OpenAliceProvisioningUpsertUserRequest{
		OperationId:       "op-mismatch",
		ExternalAccountId: "acct_mismatch",
		Email:             "first@example.com",
	}
	ctx, recorder := newOpenAliceProvisioningContext(t, req)
	OpenAliceProvisioningUpsertUser(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	req.Email = "different@example.com"
	ctx, recorder = newOpenAliceProvisioningContext(t, req)
	OpenAliceProvisioningUpsertUser(ctx)
	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"code":"operation_mismatch"`)
}

func TestOpenAliceProvisioningReclaimsStaleOperationLease(t *testing.T) {
	setupOpenAliceProvisioningControllerTestDB(t)

	req := OpenAliceProvisioningUpsertUserRequest{
		OperationId:       "op-stale",
		ExternalAccountId: "acct_stale",
	}
	payload, err := common.Marshal(req)
	require.NoError(t, err)
	op := model.ProvisioningOperation{
		OperationId:       req.OperationId,
		Action:            "user.upsert",
		Status:            model.ProvisioningOperationStatusStarted,
		ExternalAccountId: req.ExternalAccountId,
		RequestPayload:    string(payload),
		Attempts:          1,
		CreatedAt:         common.GetTimestamp() - 2*openAliceProvisioningOperationLeaseSeconds,
		UpdatedAt:         common.GetTimestamp() - 2*openAliceProvisioningOperationLeaseSeconds,
	}
	require.NoError(t, model.StartProvisioningOperation(&op))

	ctx, recorder := newOpenAliceProvisioningContext(t, req)
	OpenAliceProvisioningUpsertUser(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.NoError(t, model.DB.First(&op, op.Id).Error)
	require.Equal(t, model.ProvisioningOperationStatusSuccess, op.Status)
	require.Equal(t, 2, op.Attempts)
}

func TestOpenAliceProvisioningKeepsFreshOperationLease(t *testing.T) {
	setupOpenAliceProvisioningControllerTestDB(t)

	req := OpenAliceProvisioningUpsertUserRequest{
		OperationId:       "op-fresh",
		ExternalAccountId: "acct_fresh",
	}
	payload, err := common.Marshal(req)
	require.NoError(t, err)
	op := model.ProvisioningOperation{
		OperationId:       req.OperationId,
		Action:            "user.upsert",
		Status:            model.ProvisioningOperationStatusStarted,
		ExternalAccountId: req.ExternalAccountId,
		RequestPayload:    string(payload),
	}
	require.NoError(t, model.StartProvisioningOperation(&op))

	ctx, recorder := newOpenAliceProvisioningContext(t, req)
	OpenAliceProvisioningUpsertUser(ctx)
	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"code":"operation_in_progress"`)
}

func TestOpenAliceProvisioningUserGroupOwnsTokenExecutionPolicy(t *testing.T) {
	setupOpenAliceProvisioningControllerTestDB(t)

	upsertReq := OpenAliceProvisioningUpsertUserRequest{
		OperationId:       "op-group-user",
		ExternalAccountId: "acct_group",
		Group:             "free",
	}
	ctx, recorder := newOpenAliceProvisioningContext(t, upsertReq)
	OpenAliceProvisioningUpsertUser(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	tokenReq := OpenAliceProvisioningCreateTokenRequest{
		OperationId:       "op-group-token",
		ExternalAccountId: upsertReq.ExternalAccountId,
		ManagedKeyId:      "acct_group:desktop:1",
	}
	ctx, recorder = newOpenAliceProvisioningContext(t, tokenReq)
	OpenAliceProvisioningCreateToken(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response openAliceProvisioningAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", response.Data.Id).Update("group", "legacy-token-group").Error)

	upsertReq.OperationId = "op-group-user-upgrade"
	upsertReq.Group = "scale"
	ctx, recorder = newOpenAliceProvisioningContext(t, upsertReq)
	OpenAliceProvisioningUpsertUser(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	user, err := model.GetUserByExternalAccountId(upsertReq.ExternalAccountId, true)
	require.NoError(t, err)
	require.Equal(t, "scale", user.Group)
	var token model.Token
	require.NoError(t, model.DB.First(&token, response.Data.Id).Error)
	require.Empty(t, token.Group)

	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("group", "legacy-user-group").Error)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", token.Id).Update("group", "legacy-token-group").Error)
	ctx, recorder = newOpenAliceProvisioningContext(t, upsertReq)
	OpenAliceProvisioningUpsertUser(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	user, err = model.GetUserByExternalAccountId(upsertReq.ExternalAccountId, true)
	require.NoError(t, err)
	require.Equal(t, "scale", user.Group)
	require.NoError(t, model.DB.First(&token, response.Data.Id).Error)
	require.Empty(t, token.Group)
	op, err := model.GetProvisioningOperationByOperationId(upsertReq.OperationId)
	require.NoError(t, err)
	require.Equal(t, 2, op.Attempts)
}

func TestOpenAliceProvisioningUpdatesRateLimitPolicy(t *testing.T) {
	setupOpenAliceProvisioningControllerTestDB(t)
	common.OpenAliceManagedMode = true
	require.NoError(t, model.DB.Create(&model.Channel{
		Type:   16,
		Key:    "test-key",
		Status: common.ChannelStatusEnabled,
		Name:   "glm",
		Models: "glm-5.2",
		Group:  "default",
	}).Error)

	enabled := true
	req := OpenAliceProvisioningRateLimitPolicyRequest{
		OperationId:   "op-rate-limit-policy",
		Enabled:       &enabled,
		WindowMinutes: 2,
		Groups: map[string][2]int{
			"free":    {20, 10},
			"starter": {120, 100},
			"pro":     {600, 500},
			"scale":   {1500, 1200},
		},
		RoutableGroups: []string{"free", "starter", "pro", "scale"},
	}
	ctx, recorder := newOpenAliceProvisioningContext(t, req)
	OpenAliceProvisioningUpdateRateLimitPolicy(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	require.True(t, setting.ModelRequestRateLimitEnabled)
	require.Equal(t, 2, setting.ModelRequestRateLimitDurationMinutes)
	total, success, found := setting.GetGroupRateLimit("pro")
	require.True(t, found)
	require.Equal(t, 600, total)
	require.Equal(t, 500, success)

	var option model.Option
	require.NoError(t, model.DB.First(&option, "key = ?", "ModelRequestRateLimitGroup").Error)
	require.Contains(t, option.Value, `"pro"`)

	usableGroups := setting.GetUserUsableGroupsCopy()
	require.Equal(t, "OpenAlice managed free", usableGroups["free"])
	require.Equal(t, "OpenAlice managed pro", usableGroups["pro"])
	require.True(t, ratio_setting.ContainsGroupRatio("free"))
	require.True(t, ratio_setting.ContainsGroupRatio("pro"))
	require.Equal(t, float64(1), ratio_setting.GetGroupRatio("free"))

	var usableGroupsOption model.Option
	require.NoError(t, model.DB.First(&usableGroupsOption, "key = ?", "UserUsableGroups").Error)
	require.Contains(t, usableGroupsOption.Value, `"free"`)
	var groupRatioOption model.Option
	require.NoError(t, model.DB.First(&groupRatioOption, "key = ?", "GroupRatio").Error)
	require.Contains(t, groupRatioOption.Value, `"free"`)

	var channel model.Channel
	require.NoError(t, model.DB.First(&channel, "name = ?", "glm").Error)
	require.Contains(t, channel.GetGroups(), "default")
	require.Contains(t, channel.GetGroups(), "free")
	require.Contains(t, channel.GetGroups(), "pro")

	var freeAbility model.Ability
	require.NoError(t, model.DB.First(&freeAbility, "channel_id = ? AND model = ? AND `group` = ?", channel.Id, "glm-5.2", "free").Error)
	require.True(t, freeAbility.Enabled)
	var proAbility model.Ability
	require.NoError(t, model.DB.First(&proAbility, "channel_id = ? AND model = ? AND `group` = ?", channel.Id, "glm-5.2", "pro").Error)
	require.True(t, proAbility.Enabled)

	op, err := model.GetProvisioningOperationByOperationId("op-rate-limit-policy")
	require.NoError(t, err)
	require.Equal(t, model.ProvisioningOperationStatusSuccess, op.Status)
	require.Contains(t, op.ResponsePayload, `"window_minutes":2`)

	require.NoError(t, model.DB.Create(&model.Channel{
		Type:   16,
		Key:    "late-key",
		Status: common.ChannelStatusEnabled,
		Name:   "late-glm",
		Models: "glm-5.2",
		Group:  "default",
	}).Error)
	ctx, recorder = newOpenAliceProvisioningContext(t, req)
	OpenAliceProvisioningUpdateRateLimitPolicy(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	var lateChannel model.Channel
	require.NoError(t, model.DB.First(&lateChannel, "name = ?", "late-glm").Error)
	require.ElementsMatch(t, []string{"default", "free", "starter", "pro", "scale"}, lateChannel.GetGroups())
	var lateProAbility model.Ability
	require.NoError(t, model.DB.First(&lateProAbility, "channel_id = ? AND model = ? AND `group` = ?", lateChannel.Id, "glm-5.2", "pro").Error)
	op, err = model.GetProvisioningOperationByOperationId(req.OperationId)
	require.NoError(t, err)
	require.Equal(t, 2, op.Attempts)

	replacement := OpenAliceProvisioningRateLimitPolicyRequest{
		OperationId:   "op-rate-limit-policy-replace",
		Enabled:       &enabled,
		WindowMinutes: 1,
		Groups: map[string][2]int{
			"free":    {10, 8},
			"blocked": {1, 0},
		},
		RoutableGroups: []string{"free"},
	}
	ctx, recorder = newOpenAliceProvisioningContext(t, replacement)
	OpenAliceProvisioningUpdateRateLimitPolicy(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	require.ElementsMatch(t, []string{"blocked", "free"}, setting.GetOpenAliceManagedGroups())
	require.Equal(t, []string{"free"}, setting.GetOpenAliceRoutableGroups())
	require.False(t, ratio_setting.ContainsGroupRatio("pro"))
	require.True(t, ratio_setting.ContainsGroupRatio("blocked"))
	require.NoError(t, model.DB.First(&channel, "name = ?", "glm").Error)
	require.ElementsMatch(t, []string{"default", "free"}, channel.GetGroups())
	var removedAbilityCount int64
	require.NoError(t, model.DB.Model(&model.Ability{}).
		Where("channel_id = ? AND model = ? AND `group` IN ?", channel.Id, "glm-5.2", []string{"pro", "blocked"}).
		Count(&removedAbilityCount).Error)
	require.Zero(t, removedAbilityCount)
}

func TestOpenAliceProvisioningRejectsInvalidRateLimitPolicy(t *testing.T) {
	setupOpenAliceProvisioningControllerTestDB(t)

	req := OpenAliceProvisioningRateLimitPolicyRequest{
		OperationId:   "op-rate-limit-invalid",
		WindowMinutes: 0,
		Groups: map[string][2]int{
			"bad group": {1, 1},
		},
		RoutableGroups: []string{"bad group"},
	}
	ctx, recorder := newOpenAliceProvisioningContext(t, req)
	OpenAliceProvisioningUpdateRateLimitPolicy(ctx)
	require.Equal(t, http.StatusBadRequest, recorder.Code)

	op, err := model.GetProvisioningOperationByOperationId("op-rate-limit-invalid")
	require.NoError(t, err)
	require.Equal(t, model.ProvisioningOperationStatusFailed, op.Status)

	req = OpenAliceProvisioningRateLimitPolicyRequest{
		OperationId:   "op-rate-limit-invalid-success-cap",
		WindowMinutes: 1,
		Groups: map[string][2]int{
			"free": {3, 6},
		},
		RoutableGroups: []string{"free"},
	}
	ctx, recorder = newOpenAliceProvisioningContext(t, req)
	OpenAliceProvisioningUpdateRateLimitPolicy(ctx)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "success rate limit cannot exceed total rate limit")

	op, err = model.GetProvisioningOperationByOperationId("op-rate-limit-invalid-success-cap")
	require.NoError(t, err)
	require.Equal(t, model.ProvisioningOperationStatusFailed, op.Status)
}
