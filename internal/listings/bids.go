package listings

import (
	"greenvue/internal/db"
	"greenvue/lib"
	"greenvue/lib/errors"

	"github.com/gofiber/fiber/v2"
)

// UploadBid handles the creation of a new bid
func UploadBid(c *fiber.Ctx) error {
	client := db.GetGlobalClient()
	if client == nil {
		return errors.InternalServerError("Failed to get database client")
	}

	var bid lib.Bid
	if err := c.BodyParser(&bid); err != nil {
		return errors.BadRequest("Failed to parse bid data: " + err.Error())
	}

	data, err := client.POST("bids", bid)
	if err != nil {
		return errors.InternalServerError("Failed to store bid: " + err.Error())
	}

	return errors.SuccessResponse(c, data)
}
