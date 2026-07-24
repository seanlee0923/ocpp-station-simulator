package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"ocpp-station-simulator/backend/internal/auth"
	"ocpp-station-simulator/backend/internal/db"
)

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type meResponse struct {
	Username string `json:"username"`
	IsAdmin  bool   `json:"isAdmin"`
}

func (app *App) login(c *gin.Context) {
	var body loginRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var user db.User
	// Same "invalid username or password" response whether the username
	// doesn't exist or the password is wrong — distinguishing the two would
	// let an attacker enumerate valid usernames.
	if err := app.DB.Where("username = ?", body.Username).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}
	if !auth.VerifyPassword(user.PasswordHash, body.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}
	token, expires := app.Sessions.Issue(user.Username, user.IsAdmin)
	c.SetCookie(auth.SessionCookieName, token, int(time.Until(expires).Seconds()), "/", "", false, true)
	c.JSON(http.StatusOK, meResponse{Username: user.Username, IsAdmin: user.IsAdmin})
}

func (app *App) logout(c *gin.Context) {
	c.SetCookie(auth.SessionCookieName, "", -1, "/", "", false, true)
	c.Status(http.StatusNoContent)
}

// me lets the frontend check on load whether a session cookie is already
// valid, without needing a dedicated "am I logged in" flag in local state.
func (app *App) me(c *gin.Context) {
	c.JSON(http.StatusOK, meResponse{Username: actorFrom(c), IsAdmin: isAdminFrom(c)})
}
