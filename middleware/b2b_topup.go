package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const glimoB2BGroup = "b2b"

// B2BTopUpOnly prevents non-approved users from opening the wallet APIs or
// creating payment sessions by calling the endpoints directly. The database
// lookup is intentional: changing a user's group takes effect immediately and
// does not rely on the group cached in an older login session.
func B2BTopUpOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		group, err := model.GetUserGroup(c.GetInt("id"), true)
		if err != nil || group != glimoB2BGroup {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "B2B approval is required to access wallet and payment functions",
			})
			return
		}
		c.Next()
	}
}
