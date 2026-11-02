package main

import (
	"bytes"
	"cmp"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"maps"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"gopkg.in/ini.v1"
)

const (
	baseURL            = "https://api.raidify.app/api/public/v1"
	sixMonthsInSeconds = 15552000.0 // 180 days
	twoMonthsInSeconds = 5184000.0  // 60 days
)

// Config holds user-defined weightings and thresholds
type Config struct {
	APIKey                       string  `ini:"api_key"`
	DiscordWebhookURL            string  `ini:"discord_webhook_url"`
	MinNumRaidsAttended          int     `ini:"min_num_raids_attended"`
	MinRecentPlayerAttendance    float64 `ini:"min_recent_player_attendance"`
	MinRecentCharacterAttendance float64 `ini:"min_recent_character_attendance"`
	WeightU                      float64 `ini:"weight_U"`
	WeightL                      float64 `ini:"weight_L"`
	WeightT                      float64 `ini:"weight_T"`
	WeightR                      float64 `ini:"weight_R"`
	WeightA                      float64 `ini:"weight_A"`
}

type GenericPageResponse[T any] struct {
	Rows       []T     `json:"rows"`
	NextCursor *string `json:"nextCursor"`
}

type Me struct {
	GuildID           string    `json:"guildId"`
	GuildName         string    `json:"guildName"`
	KeyCreatedAt      time.Time `json:"keyCreatedAt"`
	KeyLabel          string    `json:"keyLabel"`
	RequestsPerMinute int       `json:"aprequestsPerMinute"`
	WishlistMax       int
}

type ItemPrio struct {
	CharacterID   string `json:"characterId"`
	CharacterName string `json:"characterName"`
	Rank          int    `json:"rank"`
}

type ItemEntry struct {
	WowItemID      int        `json:"wowItemId"`
	Name           string     `json:"name"`
	Quality        int        `json:"quality"`
	Slot           string     `json:"slot"`
	ItemLevel      int        `json:"itemLevel"`
	AllowedClasses string     `json:"allowedClasses"`
	PugExcluded    bool       `json:"pugExcluded"`
	TokenWowItemId int        `json:"tokenWowItemId"`
	RaidInstance   string     `json:"raidInstance"`
	Source         []string   `json:"bosses"`
	Note           string     `json:"note"`
	OfficerNote    string     `json:"officerNote"`
	TierLabel      string     `json:"tierLabel"`
	StandingRule   string     `json:"standingRule"`
	Prios          []ItemPrio `json:"prios"`
}

type WishlistEntry struct {
	UserID         string `json:"userId"`
	CharacterID    string `json:"characterId"`
	CharacterName  string `json:"characterName"`
	CharacterRealm string `json:"characterRealm"`
	CharacterClass string `json:"characterClass"`
	Tier           string `json:"tier"` //BiS, Upgrade, Minor upgrade, Offspec, Pass
	TierRank       int
	ListKey        string    `json:"listKey"`
	WowItemID      int       `json:"wowItemId"`
	RaidInstance   string    `json:"raidInstance"`
	Rank           int       `json:"rank"`
	RankChangedAt  time.Time `json:"rankChangedAt"`
	CreatedAt      time.Time `json:"createdAt"`
}

type LootEntry struct {
	UserID              string    `json:"userId"`
	CharacterID         string    `json:"characterId"`
	CharacterName       string    `json:"characterName"`
	CharacterRealm      string    `json:"characterRealm"`
	ExternalName        string    `json:"externalName"`
	ExternalRealm       string    `json:"externalRealm"`
	WowItemID           int       `json:"wowItemId"`
	RaidID              string    `json:"raidId"`
	AwardType           string    `json:"awardType"`
	RaidInstance        string    `json:"raidInstance"`
	AwardedAt           time.Time `json:"awardedAt"`
	Tier                string    `json:"itemTier"`
	WishlistOutstanding int       `json:"wishlistOutstanding"` //how many items the recipient was still waiting on at the moment
	WishlistRank        int       `json:"wishlistRank"`
	WishlistTier        string    `json:"wishlistTier"`  //BiS, Upgrade, Minor upgrade, Offspec, Pass
	WishlistTotal       int       `json:"wishlistTotal"` //size of wish list
}

type AttendanceEntry struct {
	OccurredAt      time.Time `json:"occurredAt"`
	UserID          string    `json:"userId"`
	CharacterID     string    `json:"characterId"`
	CharacterName   string    `json:"characterName"`
	CharacterRealm  string    `json:"characterRealm"`
	ExternalName    string    `json:"externalName"`
	ExternalRealm   string    `json:"externalRealm"`
	State           string    `json:"state"`
	Source          string    `json:"source"`
	RaidID          string    `json:"raidId"`
	RaidTitle       string    `json:"raidTitle"`
	RaidInstance    string    `json:"raidInstance"`
	RaidScheduledAt time.Time `json:"raidScheduledAt"`
}

type ULTRA struct {
	UNumBigUpgrades      int
	uNumNeedWins         int //non-wishlisted items not tagged "free" won on a Need roll
	LTimeInRank          time.Duration
	TTimeSinceBigUpgrade time.Duration
	RNumRaidsAttended    int
	RNumRaidsAvailable   int
	ARecentAttendance    float64
	CharacterAttendance  float64
	PrioEligible         bool
	Score                float64
}

// ScoredWishlist combines a wishlist entry with its calculated ULTRA score
type ScoredWishlist struct {
	WishlistEntry
	ULTRA
}

func main() {
	// 1. Parse Config
	configFileArgument := flag.String("config", "config.ini", "filename")
	discordArgument := flag.Bool("discord", true, "send to Discord")
	flag.Parse()
	configFile := *configFileArgument
	sendToDiscord := *discordArgument

	cfg, err := ini.Load(configFile)
	if err != nil {
		log.Fatalf("Failed to read %s: %v", configFile, err)
	}

	var settings Config
	if err = cfg.MapTo(&settings); err != nil {
		log.Fatalf("Failed to parse config file: %v", err)
	}

	apiKey := settings.APIKey
	if apiKey == "" {
		log.Fatalf("api_key not found in %s", configFile)
	}

	// 2. Verify Key & Fetch Guild Info
	client := &http.Client{Timeout: 15 * time.Second}

	info, err := FetchSingle[Me](client, baseURL, apiKey, "/me")
	if err != nil {
		log.Fatalf("Invalid API key or authorization error: %v", err)
	}
	log.Printf("API Key verified for guild %v. API Quota: %v.\n", info.GuildName, info.RequestsPerMinute)

	// Set maximum limit per page to minimize API network requests
	params := url.Values{}
	params.Set("limit", "500")

	// 3. Get item map
	items, err := FetchAllPages[ItemEntry](client, baseURL, apiKey, "/items", params)
	if err != nil {
		log.Fatalf("Error fetching items: %v", err)
	}
	itemMap := make(map[int]*ItemEntry, len(items))
	for i := range items {
		itemMap[items[i].WowItemID] = &items[i]
	}
	log.Printf("Parsed %v items.\n", len(itemMap))

	// 4. Get Wishlists
	wishlistEntries, err := FetchAllPages[WishlistEntry](client, baseURL, apiKey, "/wishlists", params)
	if err != nil {
		log.Fatalf("Error fetching wishlist entries: %v", err)
	}
	log.Printf("Parsed %v wish list entries.\n", len(wishlistEntries))
	// add tier ranks
	for i := range wishlistEntries {
		w := &wishlistEntries[i]
		if w.Rank > info.WishlistMax {
			info.WishlistMax = w.Rank
		}
		switch w.Tier {
		case "BiS":
			w.TierRank = 1
		case "Upgrade":
			w.TierRank = 2
		case "MinorUpgrade":
			w.TierRank = 3
		case "Offspec":
			w.TierRank = 4
		case "Pass":
			w.TierRank = 5
		}
	}

	log.Printf("Max wishlist size: %v", info.WishlistMax)

	// 5. Fetch Attendance and Loot history (scoped to past 6 months)
	sixMonthsAgoStr := time.Now().Add(-180 * 24 * time.Hour).Format(time.RFC3339)
	params.Set("from", sixMonthsAgoStr)

	lootHistory, err := FetchAllPages[LootEntry](client, baseURL, apiKey, "/loot", params)
	if err != nil {
		log.Fatalf("Error fetching loot history: %v", err)
	}
	log.Printf("Parsed %v loot history entries.\n", len(lootHistory))
	// update tier to lowercase
	for i := range lootHistory {
		l := &lootHistory[i]
		l.Tier = strings.ToLower(l.Tier)
	}

	attendanceHistory, err := FetchAllPages[AttendanceEntry](client, baseURL, apiKey, "/attendance", params)
	if err != nil {
		log.Fatalf("Error fetching attendance history: %v", err)
	}
	log.Printf("Parsed %v entries from attendance history.\n", len(attendanceHistory))

	if len(attendanceHistory) == 0 || len(lootHistory) == 0 {
		log.Println("Warning: No attendance or loot data from the past 6 months are available.")
	}

	// Index Loot and Attendance by UserID for O(1) loop lookups
	userLootMap := make(map[string][]LootEntry)
	for _, l := range lootHistory {
		if l.UserID != "" {
			userLootMap[l.UserID] = append(userLootMap[l.UserID], l)
		}
	}

	userAttendanceMap := make(map[string][]AttendanceEntry)
	for _, a := range attendanceHistory {
		if a.UserID != "" {
			userAttendanceMap[a.UserID] = append(userAttendanceMap[a.UserID], a)
		}
	}

	// 6. Filter wishlistEntries on BiS items & detect contention
	now := time.Now()
	type itemRankKey struct {
		wowItemID int
		rank      int
	}

	grouped := make(map[itemRankKey][]WishlistEntry)
	for _, entry := range wishlistEntries {
		if entry.Rank < 4 {
			key := itemRankKey{wowItemID: entry.WowItemID, rank: entry.Rank}
			grouped[key] = append(grouped[key], entry)
		}
	}

	var candidates []ScoredWishlist

	for key, entries := range grouped {
		if len(entries) < 2 {
			continue // Not contentious
		}

		item, ok := itemMap[key.wowItemID]
		if !ok {
			log.Printf("Error: wowItemID %v not found in loot table. Skipping.", key.wowItemID)
			continue
		}

		for _, entry := range entries {
			userLoot := userLootMap[entry.UserID]
			userAtt := userAttendanceMap[entry.UserID]

			ultra := calculateULTRA(
				now, entry, *item, userLoot, userAtt, attendanceHistory, settings, *info,
			)
			candidates = append(candidates, ScoredWishlist{
				WishlistEntry: entry,
				ULTRA:         ultra,
			})
		}
	}

	if len(candidates) == 0 {
		fmt.Println("No contentious wishlist items found.")
		return
	}

	// 7. Sort Output using on-the-fly Item map lookups

	slices.SortFunc(candidates, func(a, b ScoredWishlist) int {

		return cmp.Or(
			cmp.Compare(a.RaidInstance, b.RaidInstance),
			cmp.Compare(a.WowItemID, b.WowItemID),
			cmp.Compare(a.Rank, b.Rank),
			cmp.Compare(a.TierRank, b.TierRank),
			-cmp.Compare(a.Score, b.Score),
		)
	})

	// 8. Write Results to Timestamped CSV
	outputDir := "reports"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	timestamp := now.Format("20060102_150405")
	guildName := slugify(info.GuildName)
	fileName := fmt.Sprintf("ULTRA_%s_%s.csv", guildName, timestamp)
	outFilePath := filepath.Join(outputDir, fileName)
	// Write to file inside a scoped function to ensure Flush & Close finish before Discord upload
	err = func() error {
		file, err := os.Create(outFilePath)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer file.Close()

		writer := csv.NewWriter(file)
		defer writer.Flush()

		// Write Header
		writer.Write([]string{"raidInstance", "itemName",
			"characterName", "characterRealm", "characterClass", "wishlistRank", "tier",
			"needWins", "rankedDays", "lastBigWin", "numRaidsAtt", "playerAtt2M", "characterAtt2M",
			"ULTRA", "prioEligible"})

		// Write Rows
		for _, c := range candidates {
			item := itemMap[c.WowItemID]
			daysRanked := c.ULTRA.LTimeInRank.Hours() / 24
			daysSinceWin := c.ULTRA.TTimeSinceBigUpgrade.Hours() / 24
			needWins := fmt.Sprintf("WL:%v/Non-WL:%v", c.UNumBigUpgrades, c.uNumNeedWins)
			var prioEligible string
			if c.PrioEligible {
				prioEligible = "Yes"
			} else {
				prioEligible = "No"
			}
			writer.Write([]string{
				item.RaidInstance,
				item.Name,
				c.CharacterName,
				c.CharacterRealm,
				c.CharacterClass,
				strconv.Itoa(c.Rank),
				c.Tier,
				needWins,
				fmt.Sprintf("%.2fd", daysRanked),
				fmt.Sprintf("%.2fd", daysSinceWin),
				strconv.Itoa(c.RNumRaidsAttended),
				fmt.Sprintf("%.1f%%", c.ARecentAttendance*100.0),
				fmt.Sprintf("%.1f%%", c.CharacterAttendance*100.0),
				fmt.Sprintf("%.2f", c.Score*100),
				prioEligible,
			})
		}
		return nil
	}()

	if err != nil {
		log.Fatalf("Error writing CSV file: %v", err)
	}

	fmt.Printf("Successfully generated ULTRA priority list: %s\n", outFilePath)
	if sendToDiscord && settings.DiscordWebhookURL != "" {
		comment := "Automated output used by officers to generate prio lists (PLs) for contentious items."
		msg := fmt.Sprintf("📊 **ULTRA Priority Report Generated** (`%s`)\n"+comment, fileName)
		if err := SendCSVToDiscord(settings.DiscordWebhookURL, outFilePath, msg); err != nil {
			log.Printf("Failed to post to Discord: %v\n", err)
		} else {
			log.Println("Successfully posted ULTRA report to Discord!")
		}
	}
}

// ULTRA Score Calculation Logic
func calculateULTRA(
	now time.Time,
	wishlistEntry WishlistEntry,
	itemEntry ItemEntry,
	userLoot []LootEntry,
	userAtt []AttendanceEntry,
	allAttendance []AttendanceEntry,
	settings Config,
	guildInfo Me,
) (ultra ULTRA) {

	sixMonthsAgo := now.Add(-180 * 24 * time.Hour)
	twoMonthsAgo := now.Add(-60 * 24 * time.Hour)

	// --- U Score Calculation (Big upgrades) ---
	// Considers Loot awarded on a Need roll and how "big" the upgrade was according to the player.

	uScore := 1.0

	var lastBigUpgradeTime time.Time
	var numBigUpgrades int
	var numNeedWins int

	for _, l := range userLoot {
		if l.AwardType == "Mainspec" && itemEntry.RaidInstance == wishlistEntry.RaidInstance && l.AwardedAt.After(sixMonthsAgo) {
			var lootedItemValue float64
			if l.WishlistRank > 0 {
				numBigUpgrades++
				if l.AwardedAt.After(lastBigUpgradeTime) {
					lastBigUpgradeTime = l.AwardedAt
				}
				lootedItemValue = 1.0 / float64(l.WishlistRank) / 2.0
			} else if l.Tier == "free" {
				lootedItemValue = 0.0
			} else {
				lootedItemValue = 1.0 / float64(guildInfo.WishlistMax) / 2.0
				numNeedWins++
			}
			uScore -= lootedItemValue
		}
	}

	ultra.UNumBigUpgrades = numBigUpgrades
	ultra.uNumNeedWins = numNeedWins
	uScore = math.Max(0.0, uScore)

	// --- L Score Calculation (how long it's been ranked the same on their wish list) ---
	ultra.LTimeInRank = now.Sub(wishlistEntry.RankChangedAt)
	secondsRanked := ultra.LTimeInRank.Seconds()
	lScore := math.Min(1.0, math.Max(0.0, secondsRanked/sixMonthsInSeconds))
	//lScore := math.Min(1.0, secondsRanked/sixMonthsInSeconds)

	// --- T Score Calculation (time since big upgrade) ---
	var tScore float64
	if lastBigUpgradeTime.IsZero() {
		tScore = 1.0
		ultra.TTimeSinceBigUpgrade = time.Duration(sixMonthsInSeconds * float64(time.Second))
	} else {
		secondsSince := now.Sub(lastBigUpgradeTime).Seconds()
		tScore = math.Min(1.0, math.Max(0.0, secondsSince/sixMonthsInSeconds))
		//tScore = math.Min(1.0, secondsSince/sixMonthsInSeconds)
		ultra.TTimeSinceBigUpgrade = now.Sub(lastBigUpgradeTime)
	}

	// --- R Score Calculation (number of raids attended in the past 6 months or since first raid in zone, whichever is shorter) ---
	raidsAttended6M := 0
	for _, a := range userAtt {
		if a.RaidInstance == itemEntry.RaidInstance && a.OccurredAt.After(sixMonthsAgo) {
			raidsAttended6M++
		}
	}

	guildRaidDates6M := make(map[string]bool)
	for _, a := range allAttendance {
		if a.RaidInstance == itemEntry.RaidInstance && a.OccurredAt.After(sixMonthsAgo) {
			guildRaidDates6M[a.OccurredAt.Format("2006-01-02")] = true
		}
	}
	guildRaids6M := len(guildRaidDates6M)

	var rScore float64
	if guildRaids6M == 0 {
		rScore = 0.0
	} else {
		rScore = math.Min(1.0, float64(raidsAttended6M)/float64(guildRaids6M))
	}
	ultra.RNumRaidsAttended = raidsAttended6M
	ultra.RNumRaidsAvailable = guildRaids6M

	// --- A Score Calculation (Scoped strictly to raidInstance) ---
	raidsAttended2M := 0
	raidsAttendedCharacter2M := 0
	var earliestUserRaid2M time.Time

	for _, a := range userAtt {
		if a.RaidInstance == itemEntry.RaidInstance && a.OccurredAt.After(twoMonthsAgo) {
			raidsAttended2M++
			if earliestUserRaid2M.IsZero() || a.OccurredAt.Before(earliestUserRaid2M) {
				earliestUserRaid2M = a.OccurredAt
			}
			if a.CharacterID == wishlistEntry.CharacterID {
				raidsAttendedCharacter2M++
			}
		}
	}

	guildRaidDates2M := make(map[string]bool)
	for _, a := range allAttendance {
		if a.RaidInstance == itemEntry.RaidInstance && a.OccurredAt.After(twoMonthsAgo) {
			if earliestUserRaid2M.IsZero() || !a.OccurredAt.Before(earliestUserRaid2M) {
				guildRaidDates2M[a.OccurredAt.Format("2006-01-02")] = true
			}
		}
	}
	guildRaids2M := len(guildRaidDates2M)

	var aScore float64
	var recentCharacterAttendance float64

	if guildRaids2M == 0 {
		aScore = 0.0
	} else {
		aScore = math.Min(1.0, float64(raidsAttended2M)/float64(guildRaids2M))
		recentCharacterAttendance = math.Min(1.0, float64(raidsAttendedCharacter2M)/float64(guildRaids2M))
	}
	ultra.ARecentAttendance = aScore
	ultra.CharacterAttendance = recentCharacterAttendance

	// Cap L & T to R prevent abuse
	lScore = math.Min(lScore, rScore)
	tScore = math.Min(tScore, rScore)

	// Apply guild preference weights
	uScore *= settings.WeightU
	lScore *= settings.WeightL
	tScore *= settings.WeightT
	rScore *= settings.WeightR
	aScore *= settings.WeightA

	ultra.Score = roundTo3Decimals(math.Min(1.0, uScore+lScore+tScore+rScore+aScore))

	// Determine prio eligibility
	ultra.PrioEligible = (ultra.RNumRaidsAttended >= settings.MinNumRaidsAttended &&
		ultra.ARecentAttendance >= settings.MinRecentPlayerAttendance &&
		recentCharacterAttendance >= settings.MinRecentCharacterAttendance)

	return
}

func FetchSingle[T any](client *http.Client, baseURL, apiKey, endpointPath string) (*T, error) {
	req, err := http.NewRequest(http.MethodGet, baseURL+endpointPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error [%d]: %s", resp.StatusCode, string(body))
	}

	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

func FetchAllPages[T any](client *http.Client, baseURL, apiKey, endpointPath string, baseParams url.Values) ([]T, error) {
	var allRows []T
	var currentCursor *string

	if baseParams == nil {
		baseParams = url.Values{}
	}

	for {
		reqURL, err := url.Parse(baseURL + endpointPath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse endpoint path: %w", err)
		}

		q := make(url.Values)
		maps.Copy(q, baseParams)

		if currentCursor != nil && *currentCursor != "" {
			q.Set("cursor", *currentCursor)
		}

		reqURL.RawQuery = q.Encode()

		req, err := http.NewRequest(http.MethodGet, reqURL.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("failed to build request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("http request failed: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("api error [%d]: %s", resp.StatusCode, string(body))
		}

		var page GenericPageResponse[T]
		err = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to decode generic page response: %w", err)
		}

		allRows = append(allRows, page.Rows...)

		if page.NextCursor == nil || *page.NextCursor == "" {
			break
		}

		currentCursor = page.NextCursor
	}

	return allRows, nil
}

func roundTo3Decimals(val float64) float64 {
	return math.Round(val*1000) / 1000
}

var re = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(s)
	s = re.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// SendCSVToDiscord sends a CSV file to a Discord webhook with an optional text message.
func SendCSVToDiscord(webhookURL, filePath, message string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add the file payload
	part, err := writer.CreateFormFile("files[0]", filepath.Base(filePath))
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err = io.Copy(part, file); err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}

	// Add the text message payload (optional)
	if message != "" {
		if err = writer.WriteField("content", message); err != nil {
			return fmt.Errorf("failed to write message content: %w", err)
		}
	}

	if err = writer.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, webhookURL, body)
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord webhook error [%d]: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
