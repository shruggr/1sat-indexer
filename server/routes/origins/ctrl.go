package origins

import (
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/mod/onesat"
)

var ingest *idx.IngestCtx

func RegisterRoutes(r fiber.Router, ingestCtx *idx.IngestCtx) {
	ingest = ingestCtx
	r.Post("/ancestors", OriginsAncestors)
	r.Get("/ancestors/:outpoint", OriginAncestors)
	r.Post("/history", OriginsHistory)
	r.Get("/history/:outpoint", OriginHistory)
}

// @Summary Get origin history
// @Description Get the history of a specific origin by outpoint
// @Tags origins
// @Produce json
// @Param outpoint path string true "Origin outpoint"
// @Param tags query string false "Comma-separated list of tags to include (use * for all indexed tags)"
// @Success 200 {array} idx.Txo
// @Failure 500 {string} string "Internal server error"
// @Router /v5/origins/history/{outpoint} [get]
func OriginHistory(c *fiber.Ctx) error {
	outpoint := c.Params("outpoint")
	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}

	logs, err := ingest.Store.Search(c.Context(), &idx.SearchCfg{
		Keys: []string{idx.EventKey("origin", &idx.Event{
			Id:    "outpoint",
			Value: outpoint,
		})},
	})
	if err != nil {
		return err
	}

	outpoints := make([]string, 0, len(logs))
	for _, log := range logs {
		outpoints = append(outpoints, log.Member)
	}

	if txos, err := ingest.Store.LoadTxos(c.Context(), outpoints, tags, true); err != nil {
		return err
	} else {
		return c.JSON(txos)
	}
}

// @Summary Get multiple origins history
// @Description Get the history for multiple origins by outpoints
// @Tags origins
// @Accept json
// @Produce json
// @Param outpoints body []string true "Array of origin outpoints"
// @Param tags query string false "Comma-separated list of tags to include (use * for all indexed tags)"
// @Success 200 {array} idx.Txo
// @Failure 400 {string} string "Invalid request"
// @Failure 500 {string} string "Internal server error"
// @Router /v5/origins/history [post]
func OriginsHistory(c *fiber.Ctx) error {
	var reqOutpoints []string
	if err := c.BodyParser(&reqOutpoints); err != nil {
		return c.SendStatus(400)
	} else if len(reqOutpoints) == 0 {
		return c.SendStatus(400)
	}

	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}

	allOutpoints := make([]string, 0)
	for _, outpoint := range reqOutpoints {
		logs, err := ingest.Store.Search(c.Context(), &idx.SearchCfg{
			Keys: []string{idx.EventKey("origin", &idx.Event{
				Id:    "outpoint",
				Value: outpoint,
			})},
		})
		if err != nil {
			return err
		}
		for _, log := range logs {
			allOutpoints = append(allOutpoints, log.Member)
		}
	}

	if txos, err := ingest.Store.LoadTxos(c.Context(), allOutpoints, tags, true); err != nil {
		return err
	} else {
		return c.JSON(txos)
	}
}

// @Summary Get origin ancestors
// @Description Get ancestors of a specific origin (excluding the origin itself)
// @Tags origins
// @Produce json
// @Param outpoint path string true "Origin outpoint"
// @Param tags query string false "Comma-separated list of tags to include (use * for all indexed tags)"
// @Success 200 {array} idx.Txo
// @Failure 500 {string} string "Internal server error"
// @Router /v5/origins/ancestors/{outpoint} [get]
func OriginAncestors(c *fiber.Ctx) error {
	outpoint := c.Params("outpoint")

	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}

	if data, err := ingest.Store.LoadData(c.Context(), outpoint, []string{"origin"}); err != nil {
		return err
	} else if originData, ok := data["origin"]; ok && originData != nil {
		origin := onesat.Origin{}
		if err := json.Unmarshal([]byte(originData.Data.(json.RawMessage)), &origin); err != nil {
			return err
		}
		outpoint = origin.Outpoint.String()
	}

	logs, err := ingest.Store.Search(c.Context(), &idx.SearchCfg{
		Keys: []string{idx.EventKey("origin", &idx.Event{
			Id:    "outpoint",
			Value: outpoint,
		})},
	})
	if err != nil {
		return err
	}

	outpoints := make([]string, 0, len(logs))
	for _, log := range logs {
		if log.Member != outpoint {
			outpoints = append(outpoints, log.Member)
		}
	}

	if txos, err := ingest.Store.LoadTxos(c.Context(), outpoints, tags, true); err != nil {
		return err
	} else {
		return c.JSON(txos)
	}
}

// @Summary Get ancestors for multiple origins
// @Description Get ancestors for multiple origins by outpoints
// @Tags origins
// @Accept json
// @Produce json
// @Param outpoints body []string true "Array of origin outpoints"
// @Param tags query string false "Comma-separated list of tags to include (use * for all indexed tags)"
// @Success 200 {array} idx.Txo
// @Failure 400 {string} string "Invalid request"
// @Failure 500 {string} string "Internal server error"
// @Router /v5/origins/ancestors [post]
func OriginsAncestors(c *fiber.Ctx) error {
	var reqOutpoints []string
	if err := c.BodyParser(&reqOutpoints); err != nil {
		return c.SendStatus(400)
	} else if len(reqOutpoints) == 0 {
		return c.SendStatus(400)
	}

	tags := strings.Split(c.Query("tags", ""), ",")
	if len(tags) > 0 && tags[0] == "*" {
		tags = ingest.IndexedTags()
	}

	origins := make([]string, 0, len(reqOutpoints))
	outpointMap := make(map[string]struct{}, len(reqOutpoints))
	for _, outpoint := range reqOutpoints {
		if data, err := ingest.Store.LoadData(c.Context(), outpoint, []string{"origin"}); err != nil {
			return err
		} else if originData, ok := data["origin"]; ok && originData != nil {
			origin := onesat.Origin{}
			if err := json.Unmarshal([]byte(originData.Data.(json.RawMessage)), &origin); err != nil {
				return err
			}
			if origin.Outpoint != nil {
				origins = append(origins, origin.Outpoint.String())
			}
		}
	}

	allOutpoints := make([]string, 0)
	for _, outpoint := range origins {
		logs, err := ingest.Store.Search(c.Context(), &idx.SearchCfg{
			Keys: []string{idx.EventKey("origin", &idx.Event{
				Id:    "outpoint",
				Value: outpoint,
			})},
		})
		if err != nil {
			return err
		}
		for _, log := range logs {
			op := log.Member
			if _, ok := outpointMap[op]; !ok {
				outpointMap[op] = struct{}{}
				allOutpoints = append(allOutpoints, op)
			}
		}
	}

	if txos, err := ingest.Store.LoadTxos(c.Context(), allOutpoints, tags, true); err != nil {
		return err
	} else {
		return c.JSON(txos)
	}
}
