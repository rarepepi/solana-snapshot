package server

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func (s *Server) RegisterRoutes() http.Handler {
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{"https://*", "http://*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	e.GET("/", s.HelloWorldHandler)
	e.GET("/holders/:mintAddress", s.GetHoldersHandler)

	return e
}

func (s *Server) HelloWorldHandler(c echo.Context) error {
	resp := map[string]string{
		"message": "Hello World",
	}

	return c.JSON(http.StatusOK, resp)
}

func (s *Server) GetHoldersHandler(c echo.Context) error {
	mintAddress := c.Param("mintAddress")
	url := fmt.Sprintf("https://mainnet.helius-rpc.com/?api-key=%s", os.Getenv("HELIUS_API_KEY"))

	page := 1
	holderMap := make(map[string]struct {
		Owner  string
		Amount float64
	})

	totalSupply := 0.0
	totalAirdropAmount := 20000000.0 // 20 million tokens to distribute

	excludedAddresses := map[string]bool{
		"4zkYdZwM2dyT2pg5DcfcKdMPzXPJ9f7vQZrKRuXiqJaa": true,
		"CqWe7DKyRQ58U5jDRpsJe5RvwiUmuYrrnBUV64WVTX1o": true,
	}

	for {
		payload := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      "helius-test",
			"method":  "getTokenAccounts",
			"params": map[string]interface{}{
				"page":           page,
				"limit":          1000,
				"displayOptions": map[string]interface{}{},
				"mint":           mintAddress,
			},
		}

		jsonPayload, err := json.Marshal(payload)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client := &http.Client{Transport: tr}

		resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("Error: %d, %s", resp.StatusCode, resp.Status),
			})
		}

		var result struct {
			Result struct {
				TokenAccounts []struct {
					Owner  string `json:"owner"`
					Amount int64  `json:"amount"`
				} `json:"token_accounts"`
			} `json:"result"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		if result.Result.TokenAccounts == nil || len(result.Result.TokenAccounts) == 0 {
			break
		}

		for _, account := range result.Result.TokenAccounts {
			if !excludedAddresses[account.Owner] {
				amount := float64(account.Amount) / 1e9
				if amount >= 1500 {
					holderMap[account.Owner] = struct {
						Owner  string
						Amount float64
					}{
						Owner:  account.Owner,
						Amount: amount,
					}
					totalSupply += amount
				}
			}
		}

		page++
	}

	// Create CSV response
	c.Response().Header().Set("Content-Type", "text/csv")
	c.Response().Header().Set("Content-Disposition", "attachment; filename=holders.csv")

	buffer := &bytes.Buffer{}

	for _, info := range holderMap {
		airdropAmount := (info.Amount / totalSupply) * totalAirdropAmount
		buffer.WriteString(fmt.Sprintf("%s,%f\n", info.Owner, airdropAmount))
	}

	return c.Blob(http.StatusOK, "text/csv", buffer.Bytes())
}
