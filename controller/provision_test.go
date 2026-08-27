package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupProvisionTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	model.DB = db
	model.LOG_DB = db

	if err := db.AutoMigrate(&model.User{}, &model.Token{}, &model.Pool{}, &model.PoolBinding{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	if err := db.Create(&model.User{
		Id:       1,
		Username: "admin",
		Password: "hashed-password",
		Group:    "default",
		Status:   common.UserStatusEnabled,
		Role:     common.RoleAdminUser,
	}).Error; err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestResolveProvisionTokenGroup(t *testing.T) {
	if got := resolveProvisionTokenGroup(""); got != "default" {
		t.Fatalf("empty = %q", got)
	}
	if got := resolveProvisionTokenGroup("  "); got != "default" {
		t.Fatalf("blank = %q", got)
	}
	if got := resolveProvisionTokenGroup(" vip "); got != "vip" {
		t.Fatalf("vip = %q", got)
	}
}

func TestProvisionCustomerTokenPinsDefaultGroup(t *testing.T) {
	db := setupProvisionTestDB(t)
	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/provision/customer-token", map[string]any{
		"customer_uuid": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		"name":          "13800138000-aaaaaaaa",
	}, 1)

	ProvisionCustomerToken(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var token model.Token
	if err := db.Where("name = ?", "13800138000-aaaaaaaa").First(&token).Error; err != nil {
		t.Fatal(err)
	}
	if token.Group != "default" {
		t.Fatalf("group=%q want default", token.Group)
	}
}

func TestProvisionCustomerTokenUsesRequestedGroup(t *testing.T) {
	db := setupProvisionTestDB(t)
	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/provision/customer-token", map[string]any{
		"customer_uuid": "bbbbbbbb-cccc-dddd-eeee-ffffffffffff",
		"name":          "13800138001-bbbbbbbb",
		"group":         "vip",
	}, 1)

	ProvisionCustomerToken(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var token model.Token
	if err := db.Where("name = ?", "13800138001-bbbbbbbb").First(&token).Error; err != nil {
		t.Fatal(err)
	}
	if token.Group != "vip" {
		t.Fatalf("group=%q want vip", token.Group)
	}
}

func TestProvisionCustomerTokenReuseBackfillsEmptyGroup(t *testing.T) {
	db := setupProvisionTestDB(t)
	existing := &model.Token{
		UserId:         1,
		Name:           "13800138002-cccccccc",
		Key:            "existingkey1234567890abcdefghij",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    1,
		AccessedTime:   1,
		ExpiredTime:    -1,
		UnlimitedQuota: true,
		Group:          "",
	}
	if err := db.Create(existing).Error; err != nil {
		t.Fatal(err)
	}

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/provision/customer-token", map[string]any{
		"customer_uuid": "cccccccc-dddd-eeee-ffff-000000000000",
		"name":          "13800138002-cccccccc",
	}, 1)

	ProvisionCustomerToken(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Reused bool `json:"reused"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Success || !resp.Data.Reused {
		t.Fatalf("resp=%+v body=%s", resp, recorder.Body.String())
	}
	var token model.Token
	if err := db.Where("id = ?", existing.Id).First(&token).Error; err != nil {
		t.Fatal(err)
	}
	if token.Group != "default" {
		t.Fatalf("group=%q want default after reuse", token.Group)
	}
}
