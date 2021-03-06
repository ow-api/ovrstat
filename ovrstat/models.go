package ovrstat

// PlayerStats holds all stats on a specified Overwatch player
type PlayerStats struct {
	Icon             string          `json:"icon"`
	Name             string          `json:"name"`
	Level            int             `json:"level"`
	LevelIcon        string          `json:"levelIcon"`
	Prestige         int             `json:"prestige"`
	PrestigeIcon     string          `json:"prestigeIcon"`
	Rating           int             `json:"rating"`
	RatingIcon       string          `json:"ratingIcon"`
	GamesWon         int             `json:"gamesWon"`
	QuickPlayStats   statsCollection `json:"quickPlayStats"`
	CompetitiveStats statsCollection `json:"competitiveStats"`
}

// statsCollection holds a collection of stats for a particular player
type statsCollection struct {
	TopHeros    map[string]*topHeroStats `json:"topHeros"`
	CareerStats map[string]*careerStats  `json:"careerStats"`
}

// topHeroStats holds basic stats for each hero
type topHeroStats struct {
	TimePlayed          string  `json:"timePlayed"`
	GamesWon            int     `json:"gamesWon"`
	WinPercentage       int     `json:"winPercentage"`
	WeaponAccuracy      int     `json:"weaponAccuracy"`
	EliminationsPerLife float64 `json:"eliminationsPerLife"`
	MultiKillBest       int     `json:"multiKillBest"`
	ObjectiveKills      float64 `json:"objectiveKills"`
}

// careerStats holds very detailed stats for each hero
type careerStats struct {
	Assists       map[string]interface{} `json:"assists,omitempty"`
	Average       map[string]interface{} `json:"average,omitempty"`
	Best          map[string]interface{} `json:"best,omitempty"`
	Combat        map[string]interface{} `json:"combat,omitempty"`
	Deaths        map[string]interface{} `json:"deaths,omitempty"`
	HeroSpecific  map[string]interface{} `json:"heroSpecific,omitempty"`
	Game          map[string]interface{} `json:"game,omitempty"`
	MatchAwards   map[string]interface{} `json:"matchAwards,omitempty"`
	Miscellaneous map[string]interface{} `json:"miscellaneous,omitempty"`
}
