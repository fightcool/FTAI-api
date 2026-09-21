package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newUsageTemplateTestDB(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
}

func newUsageTemplateChannel(t *testing.T, channelType int, baseURL string, template *dto.ChannelUsageQueryTemplate) *model.Channel {
	t.Helper()
	channel := &model.Channel{
		Type:    channelType,
		Key:     "sk-usage-template-test",
		Name:    "usage-template-channel",
		BaseURL: &baseURL,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{UsageQueryTemplate: template})
	require.NoError(t, model.DB.Create(channel).Error)
	return channel
}

func testUsageTemplate() *dto.ChannelUsageQueryTemplate {
	return &dto.ChannelUsageQueryTemplate{
		Kind:   dto.UsageQueryTemplateKind,
		URL:    "{baseUrl}/usage",
		Method: "GET",
		Mapping: dto.UsageQueryMapping{
			Balance:   "balance",
			Remaining: "remaining",
			Used:      "usage.total.actual_cost",
			Currency:  "unit",
			Scope:     "planName",
			Available: "isValid",
		},
		Unit: "USD",
	}
}

func setTestExchangeRate(t *testing.T, rate float64) {
	t.Helper()
	previous := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = rate
	t.Cleanup(func() { operation_setting.USDExchangeRate = previous })
}

func TestFetchUsageTemplateBalanceParsesAndStoresUpstreamUsage(t *testing.T) {
	newUsageTemplateTestDB(t)
	setTestExchangeRate(t, 7.3)

	type receivedRequest struct {
		Method string
		Path   string
		Auth   string
	}
	received := make(chan receivedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- receivedRequest{Method: r.Method, Path: r.URL.Path, Auth: r.Header.Get("Authorization")}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"balance": 100.5,
			"remaining": "6.79",
			"unit": "CNY",
			"planName": "Pro",
			"isValid": true,
			"usage": {"total": {"actual_cost": 93.71}}
		}`))
	}))
	defer server.Close()

	channel := newUsageTemplateChannel(t, constant.ChannelTypeCustom, server.URL, testUsageTemplate())

	result, err := updateChannelBalance(channel)
	require.NoError(t, err)
	require.Empty(t, result.RawResponse)

	request := <-received
	require.Equal(t, http.MethodGet, request.Method)
	require.Equal(t, "/usage", request.Path)
	require.Equal(t, "Bearer sk-usage-template-test", request.Auth)

	require.InDelta(t, 6.79/7.3, result.Balance, 1e-9)
	usage := result.Usage
	require.NotNil(t, usage)
	assert.InDelta(t, 6.79, usage.Remaining, 1e-9)
	assert.InDelta(t, 100.5, usage.Balance, 1e-9)
	assert.InDelta(t, 93.71, usage.Used, 1e-9)
	assert.Equal(t, "CNY", usage.Currency)
	assert.Equal(t, "Pro", usage.Scope)
	require.NotNil(t, usage.Available)
	assert.True(t, *usage.Available)

	stored, err := model.GetChannelById(channel.Id, false)
	require.NoError(t, err)
	assert.InDelta(t, 6.79/7.3, stored.Balance, 1e-9)
}

func TestFetchUsageTemplateBalanceFallsBackToRawResponse(t *testing.T) {
	newUsageTemplateTestDB(t)
	setTestExchangeRate(t, 7.3)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"unrelated": true}`))
	}))
	defer server.Close()

	channel := newUsageTemplateChannel(t, constant.ChannelTypeCustom, server.URL, testUsageTemplate())

	result, err := updateChannelBalance(channel)
	require.NoError(t, err)
	require.NotEmpty(t, result.RawResponse)
	assert.Contains(t, result.RawResponse, "unrelated")
	assert.Zero(t, result.Balance)
	assert.Nil(t, result.Usage)

	stored, err := model.GetChannelById(channel.Id, false)
	require.NoError(t, err)
	assert.Zero(t, stored.Balance)
}

func TestFetchUsageTemplateBalanceAppliesHeaderOverride(t *testing.T) {
	newUsageTemplateTestDB(t)

	apiKey := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey <- r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"remaining": 3.25, "unit": "USD"}`))
	}))
	defer server.Close()

	channel := newUsageTemplateChannel(t, constant.ChannelTypeCustom, server.URL, &dto.ChannelUsageQueryTemplate{
		URL:     "{baseUrl}/usage",
		Mapping: dto.UsageQueryMapping{Remaining: "remaining"},
	})
	headerOverride := `{"x-api-key":"{api_key}","X-Static":"static-value"}`
	channel.HeaderOverride = &headerOverride

	result, err := updateChannelBalance(channel)
	require.NoError(t, err)
	require.InDelta(t, 3.25, result.Balance, 1e-9)
	assert.Equal(t, "sk-usage-template-test", <-apiKey)
}

func TestUpdateChannelDeepSeekBalanceConvertsCNYToUSD(t *testing.T) {
	newUsageTemplateTestDB(t)
	setTestExchangeRate(t, 7.3)

	requestPath := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"is_available": true,
			"balance_infos": [
				{"currency": "CNY", "total_balance": "6.79", "granted_balance": "0.00", "topped_up_balance": "6.79"}
			]
		}`))
	}))
	defer server.Close()

	channel := newUsageTemplateChannel(t, constant.ChannelTypeDeepSeek, server.URL, nil)

	result, err := updateChannelBalance(channel)
	require.NoError(t, err)
	require.Equal(t, "/user/balance", <-requestPath)
	require.InDelta(t, 6.79/7.3, result.Balance, 1e-9)

	stored, err := model.GetChannelById(channel.Id, false)
	require.NoError(t, err)
	assert.InDelta(t, 6.79/7.3, stored.Balance, 1e-9)
}

func TestFetchUsageTemplateBalanceCurrencyOverride(t *testing.T) {
	newUsageTemplateTestDB(t)
	setTestExchangeRate(t, 7.3)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"remaining": "6.79", "unit": "USD"}`))
	}))
	defer server.Close()

	channel := newUsageTemplateChannel(t, constant.ChannelTypeCustom, server.URL, testUsageTemplate())
	// Upstream claims USD but the wallet is actually CNY; the channel-level
	// override must win over the response label.
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		UsageQueryTemplate: testUsageTemplate(),
		BalanceCurrency:    "CNY",
	})

	result, err := updateChannelBalance(channel)
	require.NoError(t, err)
	require.InDelta(t, 6.79/7.3, result.Balance, 1e-9)
	require.NotNil(t, result.Usage)
	assert.Equal(t, "CNY", result.Usage.Currency)
}

func TestUpdateStandardChannelBalanceAppliesCurrencyOverride(t *testing.T) {
	newUsageTemplateTestDB(t)
	setTestExchangeRate(t, 7.3)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/dashboard/billing/subscription", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"billing_subscription","has_payment_method":true,"hard_limit_usd":100,"soft_limit_usd":100,"system_hard_limit_usd":100,"access_until":0}`))
	})
	mux.HandleFunc("/v1/dashboard/billing/usage", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","total_usage":730}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	channel := newUsageTemplateChannel(t, constant.ChannelTypeCustom, server.URL, nil)
	channel.SetOtherSettings(dto.ChannelOtherSettings{BalanceCurrency: "CNY"})

	result, err := updateChannelBalance(channel)
	require.NoError(t, err)
	// Raw balance 100 - 7.30 = 92.70, declared CNY → stored 92.70/7.3.
	require.InDelta(t, 92.7/7.3, result.Balance, 1e-9)
}
