package ovrstat

import (
	"encoding/json"
	"golang.org/x/net/html"
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

	// PlatformPC is a platform for PCs (mouseKeyboard in the page)
	PlatformPC = "pc"

	// PlatformConsole is a consolidated platform of all consoles
	PlatformConsole = "console"
)

var (
	// ErrPlayerNotFound is thrown when a player doesn't exist
	ErrPlayerNotFound = errors.New("Player not found")

	// ErrInvalidPlatform is thrown when the passed params are incorrect
	ErrInvalidPlatform = errors.New("Invalid platform")

	// regexpPlayerTagName matches a player's name for search rather than the full tag
	regexpPlayerTagName = regexp.MustCompile("(.*?)[#\\-]\\d+")
)

// Stats retrieves player stats
// Universal method if you don't need to differentiate it
func Stats(platformKey, tag string) (*PlayerStats, error) {
	// Do platform key mapping
	switch platformKey {
	case PlatformPC:
		platformKey = "mouseKeyboard"
	case PlatformConsole:
		platformKey = "controller"
	}

	// Parse the API response first
	var ps PlayerStats

	players, err := retrievePlayers(tag)

	if err != nil {
		return nil, err
	}

	var player *Player

	if len(players) == 0 {
		// Fall back to non-exact search to try to match the user
		tagMatch := regexpPlayerTagName.FindStringSubmatch(tag)

		if tagMatch == nil {
			return nil, ErrPlayerNotFound
		}

		players, err = retrievePlayers(tagMatch[1])

		if err != nil {
			return nil, err
		}

		for _, p := range players {
			if strings.EqualFold(tag, p.BattleTag) {
				player = &p
				break
			}
		}

		if player == nil {
			return nil, ErrPlayerNotFound
		}
	} else {
		player = &players[0]
	}

	if !player.IsPublic {
		ps.Private = true
		return &ps, nil
	}

	// Create the profile url for scraping
	profileUrl := baseURL + "/" + player.URL + "/"

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
	if res.StatusCode == http.StatusNotFound {
		return nil, ErrPlayerNotFound
	}

	ps.Name = pd.Find(".Profile-player--name").Text()

	platforms := make(map[string]Platform)

	pd.Find(".Profile-player--filters .Profile-player--filter").Each(func(i int, sel *goquery.Selection) {
		id, _ := sel.Attr("id")

		id = filterRegexp.FindStringSubmatch(id)[1]

		viewID := "." + id + "-view"

		// Using combined classes (.class.class2) we can filter out our views based on platform
		rankWrapper := pd.Find(".Profile-playerSummary--rankWrapper" + viewID)

		view := pd.Find(".Profile-view" + viewID)

		if view.Length() == 0 {
			return
		}

		platforms[id] = Platform{
			Name:        sel.Text(),
			RankWrapper: rankWrapper,
			ProfileView: view,
		}
	})

	platform, exists := platforms[platformKey]

	if !exists {
		return nil, ErrInvalidPlatform
	}

	// Scrapes all stats for the passed user and sets struct member data
	parseGeneralInfo(platform, pd.Find(".Profile-masthead").First(), &ps)

	parseDetailedStats(platform, ".quickPlay-view", &ps.QuickPlayStats.StatsCollection)
	parseDetailedStats(platform, ".competitive-view", &ps.CompetitiveStats.StatsCollection)

	competitiveSeason, _ := pd.Find("[data-latestherostatrankseasonow2]").Attr("data-latestherostatrankseasonow2")

	if competitiveSeason != "" {
		competitiveSeason, _ := strconv.Atoi(competitiveSeason)

		ps.CompetitiveStats.Season = &competitiveSeason
	}

	addGameStats(&ps, &ps.QuickPlayStats.StatsCollection)
	addGameStats(&ps, &ps.CompetitiveStats.StatsCollection)

	return &ps, nil
}

func addGameStats(ps *PlayerStats, statsCollection *StatsCollection) {
	if heroStats, ok := statsCollection.CareerStats["allHeroes"]; ok {
		if gamesPlayed, ok := heroStats.Game["gamesPlayed"]; ok {
			ps.GamesPlayed += gamesPlayed.(int)
		}

		if gamesWon, ok := heroStats.Game["gamesWon"]; ok {
			ps.GamesWon += gamesWon.(int)
		}

		if gamesLost, ok := heroStats.Game["gamesLost"]; ok {
			ps.GamesLost += gamesLost.(int)
		}
	}
}

func retrievePlayers(tag string) ([]Player, error) {
	// Perform api request
	var platforms []Player

	apires, err := http.Get(apiURL + url.PathEscape(tag))

	if err != nil {
		return nil, errors.Wrap(err, "Failed to perform platform API request")
	}

	defer apires.Body.Close()

	// Decode received JSON
	if err := json.NewDecoder(apires.Body).Decode(&platforms); err != nil {
		return nil, errors.Wrap(err, "Failed to decode platform API response")
	}

	return platforms, nil
}

var (
	endorsementRegexp = regexp.MustCompile("/(\\d+)-([a-z0-9]+)\\.svg")
	rankRegexp        = regexp.MustCompile("([a-zA-Z0-9]+)Tier-([a-z\\d]+)\\.(svg|png)")
	divisionRegexp    = regexp.MustCompile("TierDivision_(\\d+)-([a-z\\d]+)\\.(svg|png)")
	filterRegexp      = regexp.MustCompile("^([a-zA-Z]+)Filter$")
)

// populateGeneralInfo extracts the users general info and returns it in a
// PlayerStats struct
func parseGeneralInfo(platform Platform, s *goquery.Selection, ps *PlayerStats) {
	// Populates all general player information
	ps.Icon, _ = s.Find(".Profile-player--portrait").Attr("src")
	ps.EndorsementIcon, _ = s.Find(".Profile-playerSummary--endorsement").Attr("src")
	ps.Endorsement, _ = strconv.Atoi(endorsementRegexp.FindStringSubmatch(ps.EndorsementIcon)[1])

	// Parse Endorsement Icon path (/svg?path=)
	if strings.Index(ps.EndorsementIcon, "/svg") == 0 {
		q, err := url.ParseQuery(ps.EndorsementIcon[strings.Index(ps.EndorsementIcon, "?")+1:])

		if err == nil && q.Get("path") != "" {
			ps.EndorsementIcon = q.Get("path")
		}
	}

	// Ratings
	// Note that .is-active is the default platform
	// Note that this should have error handling! This is bad code!
	platform.RankWrapper.Find("div.Profile-playerSummary--roleWrapper").Each(func(i int, sel *goquery.Selection) {
		// Rank selections.
		roleIcon, found := sel.Find("img").Attr("src")

		if !found {
			return
		}

		// Format is /(offense|support|...)-HEX.(png|svg)
		// svg is in another tag, png is in the img tag
		role := path.Base(roleIcon)
		role = role[0:strings.Index(role, "-")]

		rankIcons := sel.Find("img.Profile-playerSummary--rank")

		if rankIcons.Length() < 2 {
			return
		}

		rankIcon, found := findAttribute("src", rankIcons.Get(0))

		if !found {
			return
		}

		divisionIcon, found := findAttribute("src", rankIcons.Get(1))

		if !found {
			return
		}

		rankInfo := rankRegexp.FindStringSubmatch(rankIcon)
		divisionInfo := divisionRegexp.FindStringSubmatch(divisionIcon)
		tier, _ := strconv.Atoi(divisionInfo[1])

		ps.Ratings = append(ps.Ratings, Rating{
			Group:        rankInfo[1],
			Tier:         tier,
			Role:         role,
			RoleIcon:     roleIcon,
			RankIcon:     rankIcon,
			DivisionIcon: divisionIcon,
		})
	})
}

func findAttribute(name string, node *html.Node) (string, bool) {
	if ptr := getAttributePtr(name, node); ptr != nil {
		return ptr.Val, true
	}

	return "", false
}

func getAttributePtr(attrName string, n *html.Node) *html.Attribute {
	if n == nil {
		return nil
	}

	for i, a := range n.Attr {
		if a.Key == attrName {
			return &n.Attr[i]
		}
	}
	return nil
}

// Private function to get the specified attribute's value from a node.
func getAttributeValue(attrName string, n *html.Node) (val string, exists bool) {
	if a := getAttributePtr(attrName, n); a != nil {
		val = a.Val
		exists = true
	}
	return
}

// parseDetailedStats populates the passed stats collection with detailed statistics
func parseDetailedStats(platform Platform, playMode string, sc *StatsCollection) {
	sc.TopHeroes = parseHeroStats(platform.ProfileView.Find(".Profile-heroSummary--view" + playMode))
	sc.CareerStats = parseCareerStats(platform.ProfileView.Find(".stats" + playMode))
}

// parseHeroStats : Parses stats for each individual hero and returns a map
func parseHeroStats(heroStatsSelector *goquery.Selection) map[string]*TopHeroStats {
	bhsMap := make(map[string]*TopHeroStats)
	categoryMap := make(map[string]string)

	heroStatsSelector.Find(".Profile-dropdown option").Each(func(i int, sel *goquery.Selection) {
		optionName := sel.Text()
		optionVal, _ := sel.Attr("value")

		categoryMap[optionVal] = cleanJSONKey(optionName)
	})

	heroStatsSelector.Find("div.Profile-progressBars").Each(func(i int, heroGroupSel *goquery.Selection) {
		categoryID, _ := heroGroupSel.Attr("data-category-id")
		categoryID = categoryMap[categoryID]

		heroGroupSel.Find(".Profile-progressBar").Each(func(i2 int, statSel *goquery.Selection) {
			heroName := cleanJSONKey(statSel.Find(".Profile-progressBar-title").Text())
			statVal := statSel.Find(".Profile-progressBar-description").Text()

			// Creates hero map if it doesn't exist
			if bhsMap[heroName] == nil {
				bhsMap[heroName] = new(TopHeroStats)
			}

			// Sets hero stats based on stat category type
			switch categoryID {
			case "timePlayed":
				bhsMap[heroName].TimePlayed = statVal
			case "gamesWon":
				bhsMap[heroName].GamesWon, _ = strconv.Atoi(statVal)
			case "weaponAccuracy":
				bhsMap[heroName].WeaponAccuracy, _ = strconv.Atoi(strings.Replace(statVal, "%", "", -1))
			case "criticalHitAccuracy":
				bhsMap[heroName].CriticalHitAccuracy, _ = strconv.Atoi(strings.Replace(statVal, "%", "", -1))
			case "eliminationsPerLife":
				bhsMap[heroName].EliminationsPerLife, _ = strconv.ParseFloat(statVal, 64)
			case "multikillBest":
				bhsMap[heroName].MultiKillBest, _ = strconv.Atoi(statVal)
			case "objectiveKills":
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
	careerStatsSelector.Find(".Profile-dropdown option").Each(func(i int, heroSel *goquery.Selection) {
		heroVal, _ := heroSel.Attr("value")
		heroMap[heroVal] = heroSel.Text()
	})

	// Iterates over every hero div
	careerStatsSelector.Find(".stats-container").Each(func(i int, heroStatsSel *goquery.Selection) {
		classAttributes, _ := heroStatsSel.Attr("class")

		var currentHeroOption string

		for _, class := range strings.Fields(classAttributes) {
			if !strings.HasPrefix(class, "option-") {
				continue
			}

			currentHeroOption = class[strings.Index(class, "-")+1:]
		}

		currentHero, exists := heroMap[currentHeroOption]

		if currentHeroOption == "" || !exists {
			return
		}

		currentHero = cleanJSONKey(currentHero)

		// Iterates over every stat box
		heroStatsSel.Find("div.category").Each(func(i2 int, statBoxSel *goquery.Selection) {
			statType := statBoxSel.Find(".header p").Text()
			statType = cleanJSONKey(statType)

			// Iterates over stat row
			statBoxSel.Find(".stat-item").Each(func(i3 int, statSel *goquery.Selection) {
				statKey := transformKey(cleanJSONKey(statSel.Find(".name").Text()))
				statVal := strings.Replace(statSel.Find(".value").Text(), ",", "", -1) // Removes commas from 1k+ values
				statVal = strings.TrimSpace(statVal)

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
				}
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
