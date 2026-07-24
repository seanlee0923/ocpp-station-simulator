package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ocpp-station-simulator/backend/internal/auth"
	"ocpp-station-simulator/backend/internal/db"
)

type userResponse struct {
	ID        uint64    `json:"id"`
	Username  string    `json:"username"`
	IsAdmin   bool      `json:"isAdmin"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
}

func toUserResponse(user db.User) userResponse {
	return userResponse{ID: user.ID, Username: user.Username, IsAdmin: user.IsAdmin, CreatedBy: user.CreatedBy, CreatedAt: user.CreatedAt}
}

func (app *App) listUsers(c *gin.Context) {
	var rows []db.User
	if err := app.DB.Order("created_at desc").Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	result := make([]userResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, toUserResponse(row))
	}
	c.JSON(http.StatusOK, result)
}

type createUserRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required,min=8"`
}

// createUser only ever creates a non-admin account: this app has no UI path
// to create another admin (see plan) — admins only come from the env-var
// bootstrap (internal/auth/bootstrap.go).
func (app *App) createUser(c *gin.Context) {
	var body createUserRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	user := db.User{Username: body.Username, PasswordHash: hash, IsAdmin: false, CreatedBy: actorFrom(c)}
	if err := app.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "username already exists"})
		return
	}
	c.JSON(http.StatusCreated, toUserResponse(user))
}

func (app *App) deleteUser(c *gin.Context) {
	var target db.User
	if err := app.DB.First(&target, "id = ?", c.Param("id")).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if target.IsAdmin {
		var adminCount int64
		app.DB.Model(&db.User{}).Where("is_admin = ?", true).Count(&adminCount)
		if adminCount <= 1 {
			c.JSON(http.StatusConflict, gin.H{"error": "cannot delete the last admin account"})
			return
		}
	}
	app.DB.Delete(&target)
	c.Status(http.StatusNoContent)
}
