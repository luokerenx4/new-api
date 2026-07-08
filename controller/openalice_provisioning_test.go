package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
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
	Id          int    `json:"id"`
	UserId      int    `json:"user_id"`
	Key         string `json:"key"`
	KeyPreview  string `json:"key_preview"`
	RemainQuota int    `json:"remain_quota"`
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

	tokenReq := OpenAliceProvisioningCreateTokenRequest{
		OperationId:       "op-token-create",
		ExternalAccountId: "acct_test_1",
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
	require.Equal(t, 100, response.Data.RemainQuota)

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
}

func TestOpenAliceProvisioningRejectsDuplicateOperationId(t *testing.T) {
	setupOpenAliceProvisioningControllerTestDB(t)

	req := OpenAliceProvisioningUpsertUserRequest{
		OperationId:       "op-duplicate",
		ExternalAccountId: "acct_dup",
	}
	ctx, recorder := newOpenAliceProvisioningContext(t, req)
	OpenAliceProvisioningUpsertUser(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	ctx, recorder = newOpenAliceProvisioningContext(t, req)
	OpenAliceProvisioningUpsertUser(ctx)
	require.Equal(t, http.StatusConflict, recorder.Code)
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
}

func TestOpenAliceProvisioningRejectsInvalidRateLimitPolicy(t *testing.T) {
	setupOpenAliceProvisioningControllerTestDB(t)

	req := OpenAliceProvisioningRateLimitPolicyRequest{
		OperationId:   "op-rate-limit-invalid",
		WindowMinutes: 0,
		Groups: map[string][2]int{
			"bad group": {1, 1},
		},
	}
	ctx, recorder := newOpenAliceProvisioningContext(t, req)
	OpenAliceProvisioningUpdateRateLimitPolicy(ctx)
	require.Equal(t, http.StatusBadRequest, recorder.Code)

	op, err := model.GetProvisioningOperationByOperationId("op-rate-limit-invalid")
	require.NoError(t, err)
	require.Equal(t, model.ProvisioningOperationStatusFailed, op.Status)
}
