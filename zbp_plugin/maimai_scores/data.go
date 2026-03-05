// custom/maimai_scores/data.go
package maimai_scores

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// SongInfo 结构体现在包含 ID 和别名列表
type SongInfo struct {
	Name    string   `json:"name"`
	Version int      `json:"version"`
	Aliases []string `json:"aliases"`
}

// SongDatabase 结构体现在是一个map，键是歌曲ID
type SongDatabase map[string]SongInfo

// PlayerScoresDatabase 新结构：map[用户ID]map[歌曲ID]ScoreEntry
type PlayerScoresDatabase map[string]map[string]ScoreEntry

type ScoreEntry struct {
	AchievementRate float64 `json:"achievement_rate"`
	Timestamp       int64   `json:"timestamp"`
}

var (
	scoresMutex = &sync.RWMutex{}
	songDB      SongDatabase
	allSongsDB  SongDatabase
)

// LoadSongDatabase 加载歌曲数据库到内存中
func LoadSongDatabase(filename string) (SongDatabase, error) {
	path := engine.DataFolder() + filename
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var db SongDatabase
	if err := json.Unmarshal(file, &db); err != nil {
		return nil, fmt.Errorf("解析 %s 失败: %w", filename, err)
	}
	return db, nil
}

// LoadPlayerScores 加载玩家分数数据库
func LoadPlayerScores() (PlayerScoresDatabase, error) {
	scoresMutex.RLock()
	defer scoresMutex.RUnlock()

	path := engine.DataFolder() + "player_scores.json"
	file, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(PlayerScoresDatabase), nil
	}
	if err != nil {
		return nil, err
	}
	var scores PlayerScoresDatabase
	if len(file) == 0 {
		return make(PlayerScoresDatabase), nil
	}
	if err := json.Unmarshal(file, &scores); err != nil {
		return nil, err
	}
	return scores, nil
}

// SavePlayerScores 保存玩家分数数据库
func SavePlayerScores(scores PlayerScoresDatabase) error {
	scoresMutex.Lock()
	defer scoresMutex.Unlock()

	path := engine.DataFolder() + "player_scores.json"
	data, err := json.MarshalIndent(scores, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}