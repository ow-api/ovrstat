package ovrstat

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"github.com/pkg/errors"
)

const (
	baseURL = "https://overwatch.blizzard.com/en-us/career"

	apiURL = "https://overwatch.blizzard.com/en-us/search/account-by-name/"

	// PlatformXBL is platform : XBOX
	PlatformXBL = "xbl"

	// PlatformPSN is the platform : Playstation Network
	PlatformPSN = "psn"

	// PlatformPC is the platform : PC
	PlatformPC = "pc"

	PlatformNS = "nintendo-switch"
)

var (
	// ErrPlayerNotFound is thrown when a player doesn't exist
	ErrPlayerNotFound = errors.New("Player not found")

	// ErrInvalidPlatform is thrown when the passed params are incorrect
	ErrInvalidPlatform = errors.New("Invalid platform")
)

// Stats retrieves player stats
// Universal method if you don't need to differentiate it
func Stats(tag string) (*PlayerStats, error) {
	// Create the profile url for scraping
	profileUrl := baseURL + "/" + strings.Replace(tag, "#", "-", -1) + "/"

	log.Println("Profile URL", profileUrl)

	// Perform the stats request and decode the response
	res, err := http.Get(profileUrl)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to retrieve profile")
	}
	defer res.Body.Close()

	// Parses the stats request into a goquery document
	pd, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create goquery document")
	}

	// Checks if profile not found, site still returns 200 in this case
	if pd.Find("[slot=heading]").First().Text() == "Page Not Found" {
		return nil, ErrPlayerNotFound
	}

	// Scrapes all stats for the passed user and sets struct member data
	ps := parseGeneralInfo(pd.Find(".Profile-masthead").First())

	// Perform api request
	var platforms []Platform

	apires, err := http.Get(apiURL + url.PathEscape(tag))
	if err != nil {
		return nil, errors.Wrap(err, "Failed to perform platform API request")
	}
	defer apires.Body.Close()

	// Decode received JSON
	if err := json.NewDecoder(apires.Body).Decode(&platforms); err != nil {
		return nil, errors.Wrap(err, "Failed to decode platform API response")
	}

	// Single, exact result
	//p := platforms[0]

	//ps.Name = p.Name
	//ps.Prestige = int(math.Floor(float64(p.PlayerLevel) / 100))

	if !platforms[0].IsPublic {
		ps.Private = true
		return &ps, nil
	}

	ps.Name = pd.Find(".Profile-player--name").Text()

	log.Println("Parsing detailed stats")

	parseDetailedStats(pd.Find("div#quickplay").First(), &ps.QuickPlayStats.StatsCollection)
	parseDetailedStats(pd.Find("div#competitive").First(), &ps.CompetitiveStats.StatsCollection)

	competitiveSeason, _ := pd.Find("div[data-competitive-season]").Attr("data-competitive-season")

	if competitiveSeason != "" {
		competitiveSeason, _ := strconv.Atoi(competitiveSeason)

		ps.CompetitiveStats.Season = &competitiveSeason
	}

	return &ps, nil
}

var (
	endorsementRegexp = regexp.MustCompile("/(\\d+)-([a-z0-9]+)\\.svg")
	rankRegexp        = regexp.MustCompile("([a-zA-Z0-9]+)Tier-(\\d)-([a-z\\d]+)\\.(svg|png)")
)

// populateGeneralInfo extracts the users general info and returns it in a
// PlayerStats struct
func parseGeneralInfo(s *goquery.Selection) PlayerStats {
	var ps PlayerStats

	// Populates all general player information
	ps.Icon, _ = s.Find(".Profile-player--portrait").Attr("src")
	ps.Level, _ = strconv.Atoi(s.Find("div.player-level div.u-vertical-center").First().Text())
	ps.LevelIcon, _ = s.Find("div.player-level").Attr("style")
	ps.PrestigeIcon, _ = s.Find("div.player-rank").Attr("style")
	ps.EndorsementIcon, _ = s.Find(".Profile-playerSummary--endorsement").Attr("src")
	ps.Endorsement, _ = strconv.Atoi(endorsementRegexp.FindStringSubmatch(ps.EndorsementIcon)[1])

	// Parse Endorsement Icon path (/svg?path=)
	if strings.Index(ps.EndorsementIcon, "/svg") == 0 {
		q, err := url.ParseQuery(ps.EndorsementIcon[strings.Index(ps.EndorsementIcon, "?")+1:])

		if err == nil && q.Get("path") != "" {
			ps.EndorsementIcon = q.Get("path")
		}
	}

	// Ratings.
	s.Find("div.Profile-playerSummary--rankWrapper div.Profile-playerSummary--roleWrapper").Each(func(i int, sel *goquery.Selection) {
		// Rank selections.

		roleIcon, _ := sel.Find("div.Profile-playerSummary--role img").Attr("src")
		// Format is /(offense|support|...)-HEX.svg
		role := path.Base(roleIcon)
		role = role[0:strings.Index(role, "-")]
		rankIcon, _ := sel.Find("img.Profile-playerSummary--rank").Attr("src")

		rankInfo := rankRegexp.FindStringSubmatch(rankIcon)
		level, _ := strconv.Atoi(rankInfo[2])

		ps.Ratings = append(ps.Ratings, Rating{
			Group:    rankInfo[1],
			Level:    level,
			Role:     role,
			RoleIcon: roleIcon,
			RankIcon: rankIcon,
		})
	})

	ps.GamesWon, _ = strconv.Atoi(strings.Replace(s.Find("div.masthead p.masthead-detail.h4 span").Text(), " games won", "", -1))

	return ps
}

// parseDetailedStats populates the passed stats collection with detailed statistics
func parseDetailedStats(playModeSelector *goquery.Selection, sc *StatsCollection) {
	sc.TopHeroes = parseHeroStats(playModeSelector.Find("div.progress-category").Parent())
	sc.CareerStats = parseCareerStats(playModeSelector.Find("div.js-stats").Parent())
}

// parseHeroStats : Parses stats for each individual hero and returns a map
func parseHeroStats(heroStatsSelector *goquery.Selection) map[string]*TopHeroStats {
	bhsMap := make(map[string]*TopHeroStats)

	heroStatsSelector.Find("div.progress-category").Each(func(i int, heroGroupSel *goquery.Selection) {
		categoryID, _ := heroGroupSel.Attr("data-category-id")
		categoryID = strings.Replace(categoryID, "0x0860000000000", "", -1)
		heroGroupSel.Find("div.ProgressBar").Each(func(i2 int, statSel *goquery.Selection) {
			heroName := cleanJSONKey(statSel.Find("div.ProgressBar-title").Text())
			statVal := statSel.Find("div.ProgressBar-description").Text()

			// Creates hero map if it doesn't exist
			if bhsMap[heroName] == nil {
				bhsMap[heroName] = new(TopHeroStats)
			}

			// Sets hero stats based on stat category type
			switch categoryID {
			case "021":
				bhsMap[heroName].TimePlayed = statVal
			case "039":
				bhsMap[heroName].GamesWon, _ = strconv.Atoi(statVal)
			case "3D1":
				bhsMap[heroName].WinPercentage, _ = strconv.Atoi(strings.Replace(statVal, "%", "", -1))
			case "02F":
				bhsMap[heroName].WeaponAccuracy, _ = strconv.Atoi(strings.Replace(statVal, "%", "", -1))
			case "3D2":
				bhsMap[heroName].EliminationsPerLife, _ = strconv.ParseFloat(statVal, 64)
			case "346":
				bhsMap[heroName].MultiKillBest, _ = strconv.Atoi(statVal)
			case "31C":
				bhsMap[heroName].ObjectiveKills, _ = strconv.ParseFloat(statVal, 64)
			}
		})
	})
	return bhsMap
}

// parseCareerStats
func parseCareerStats(careerStatsSelector *goquery.Selection) map[string]*CareerStats {
	csMap := make(map[string]*CareerStats)
	heroMap := make(map[string]string)

	// Populates tempHeroMap to match hero ID to name in second scrape
	careerStatsSelector.Find("select option").Each(func(i int, heroSel *goquery.Selection) {
		heroVal, _ := heroSel.Attr("value")
		heroMap[heroVal] = heroSel.Text()
	})

	// Iterates over every hero div
	careerStatsSelector.Find("div.row.js-stats").Each(func(i int, heroStatsSel *goquery.Selection) {
		currentHero, _ := heroStatsSel.Attr("data-category-id")
		currentHero = cleanJSONKey(heroMap[currentHero])

		// Iterates over every stat box
		heroStatsSel.Find("div.card-stat-block-container").Each(func(i2 int, statBoxSel *goquery.Selection) {
			statType := statBoxSel.Find(".stat-title").Text()
			statType = cleanJSONKey(statType)

			// Iterates over stat row
			statBoxSel.Find("table.DataTable tbody tr").Each(func(i3 int, statSel *goquery.Selection) {

				// Iterates over every stat td
				statKey := ""
				statVal := ""
				statSel.Find("td").Each(func(i4 int, statKV *goquery.Selection) {
					switch i4 {
					case 0:
						statKey = transformKey(cleanJSONKey(statKV.Text()))
					case 1:
						statVal = strings.Replace(statKV.Text(), ",", "", -1) // Removes commas from 1k+ values

						// Creates stat map if it doesn't exist
						if csMap[currentHero] == nil {
							csMap[currentHero] = new(CareerStats)
						}

						// Switches on type, creating category stat maps if exists (will omitempty on json marshal)
						switch statType {
						case "assists":
							if csMap[currentHero].Assists == nil {
								csMap[currentHero].Assists = make(map[string]interface{})
							}
							csMap[currentHero].Assists[statKey] = parseType(statVal)
						case "average":
							if csMap[currentHero].Average == nil {
								csMap[currentHero].Average = make(map[string]interface{})
							}
							csMap[currentHero].Average[statKey] = parseType(statVal)
						case "best":
							if csMap[currentHero].Best == nil {
								csMap[currentHero].Best = make(map[string]interface{})
							}
							csMap[currentHero].Best[statKey] = parseType(statVal)
						case "combat":
							if csMap[currentHero].Combat == nil {
								csMap[currentHero].Combat = make(map[string]interface{})
							}
							csMap[currentHero].Combat[statKey] = parseType(statVal)
						case "deaths":
							if csMap[currentHero].Deaths == nil {
								csMap[currentHero].Deaths = make(map[string]interface{})
							}
							csMap[currentHero].Deaths[statKey] = parseType(statVal)
						case "heroSpecific":
							if csMap[currentHero].HeroSpecific == nil {
								csMap[currentHero].HeroSpecific = make(map[string]interface{})
							}
							csMap[currentHero].HeroSpecific[statKey] = parseType(statVal)
						case "game":
							if csMap[currentHero].Game == nil {
								csMap[currentHero].Game = make(map[string]interface{})
							}
							csMap[currentHero].Game[statKey] = parseType(statVal)
						case "matchAwards":
							if csMap[currentHero].MatchAwards == nil {
								csMap[currentHero].MatchAwards = make(map[string]interface{})
							}
							csMap[currentHero].MatchAwards[statKey] = parseType(statVal)
						case "miscellaneous":
							if csMap[currentHero].Miscellaneous == nil {
								csMap[currentHero].Miscellaneous = make(map[string]interface{})
							}
							csMap[currentHero].Miscellaneous[statKey] = parseType(statVal)
						}
					}
				})
			})
		})
	})
	return csMap
}

func parseType(val string) interface{} {
	i, err := strconv.Atoi(val)
	if err == nil {
		return i
	}
	f, err := strconv.ParseFloat(val, 64)
	if err == nil {
		return f
	}
	return val
}

var (
	keyReplacer = strings.NewReplacer("-", " ", ".", " ", ":", " ", "'", "", "ú", "u", "ö", "o")
)

// cleanJSONKey
func cleanJSONKey(str string) string {
	// Removes localization rubish
	if strings.Contains(str, "} other {") {
		re := regexp.MustCompile("{count, plural, one {.+} other {(.+)}}")
		if len(re.FindStringSubmatch(str)) == 2 {
			otherForm := re.FindStringSubmatch(str)[1]
			str = re.ReplaceAllString(str, otherForm)
		}
	}

	str = keyReplacer.Replace(str) // Removes all dashes, dots, and colons from titles
	str = strings.ToLower(str)
	str = strings.Title(str)                // Uppercases lowercase leading characters
	str = strings.Replace(str, " ", "", -1) // Removes Spaces
	for i, v := range str {                 // Lowercases initial character
		return string(unicode.ToLower(v)) + str[i+1:]
	}
	return ""
}
