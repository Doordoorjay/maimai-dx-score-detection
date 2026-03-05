// custom/maimai_scores/logic.go
package maimai_scores

import (
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/schollz/closestmatch"
)

// ParseAchievementRate 从 OCR 识别出的达成率文本中，精确提取出数字。
func ParseAchievementRate(text string) (float64, error) {
	text = strings.ReplaceAll(text, "S", "5")
	text = strings.ReplaceAll(text, " ", "")
	re := regexp.MustCompile(`(\d{1,3}\.\d{4})`)
	matches := re.FindStringSubmatch(text)
	if len(matches) < 2 {
		return 0, fmt.Errorf("未能在 '%s' 中找到达成率格式", text)
	}
	rate, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, fmt.Errorf("解析达成率 '%s' 失败: %v", matches[1], err)
	}
	return rate, nil
}

// CorrectSongTitleAndGetID 严格按照 “主库精确 -> 备库精确 -> 主库模糊 -> 备库模糊” 的顺序进行搜索
func CorrectSongTitleAndGetID(ocrText string) (songID string, songTitle string, isFallback bool, err error) {
	// --- 阶段一: 主数据库 - 精确匹配 ---
	for id, song := range songDB {
		if song.Name == ocrText {
			return id, song.Name, false, nil
		}
		for _, alias := range song.Aliases {
			if alias == ocrText {
				return id, alias, false, nil
			}
		}
	}

	// --- 阶段二: 回退数据库 - 精确匹配 ---
	if len(allSongsDB) > 0 {
		for id, song := range allSongsDB {
			if song.Name == ocrText {
				return id, song.Name, true, nil
			}
		}
	}

	// --- 阶段三: 主数据库 - 模糊匹配 ---
	var mainDbTitles []string
	titleToIDMain := make(map[string]string)
	for id, song := range songDB {
		if _, exists := titleToIDMain[song.Name]; !exists {
			mainDbTitles = append(mainDbTitles, song.Name)
			titleToIDMain[song.Name] = id
		}
		for _, alias := range song.Aliases {
			if _, exists := titleToIDMain[alias]; !exists {
				mainDbTitles = append(mainDbTitles, alias)
				titleToIDMain[alias] = id
			}
		}
	}
	if len(mainDbTitles) > 0 {
		cmMain := closestmatch.New(mainDbTitles, []int{2, 3, 4})
		bestMatchMain := cmMain.Closest(ocrText)
		if bestMatchMain != "" {
			id := titleToIDMain[bestMatchMain]
			return id, bestMatchMain, false, nil
		}
	}

	// --- 阶段四: 回退数据库 - 模糊匹配 ---
	if len(allSongsDB) > 0 {
		var allDbTitles []string
		titleToIDAll := make(map[string]string)
		for id, song := range allSongsDB {
			if _, exists := titleToIDAll[song.Name]; !exists {
				allDbTitles = append(allDbTitles, song.Name)
				titleToIDAll[song.Name] = id
			}
		}
		if len(allDbTitles) > 0 {
			cmAll := closestmatch.New(allDbTitles, []int{2, 3, 4})
			bestMatchAll := cmAll.Closest(ocrText)
			if bestMatchAll != "" {
				id := titleToIDAll[bestMatchAll]
				return id, bestMatchAll, true, nil
			}
		}
	}

	// --- 如果都找不到 ---
	return "", "", false, fmt.Errorf("无法在歌曲库中匹配到 '%s'", ocrText)
}

// GenerateResponseAndSaveScore 负责比较分数、生成回复（找到歌曲后，会自动优先使用别名回复）。
func GenerateResponseAndSaveScore(userID, songID, songTitle string, isFallback bool, newRate float64) (string, bool) {
	scores, err := LoadPlayerScores()
	if err != nil {
		return fmt.Sprintf("[Maimai Scores] 读取成绩数据库失败: %v", err), false
	}

	if _, ok := scores[userID]; !ok {
		scores[userID] = make(map[string]ScoreEntry)
	}
	oldScore, scoreExists := scores[userID][songID]

	var displayTitle string
	if isFallback {
		displayTitle = songTitle
	} else {
		songInfo, ok := songDB[songID]
		if ok && len(songInfo.Aliases) > 0 {
			// 如果歌曲有别名，随机选择一个用于显示
			displayTitle = songInfo.Aliases[rand.Intn(len(songInfo.Aliases))]
		} else if ok {
			displayTitle = songInfo.Name
		} else {
			displayTitle = songTitle
		}
	}

	shouldSave := true
	var response string

	// 当分数变低时 newRate < oldScore.AchievementRate
	lowerScoreResponses := []string{
		"怎么回事？上次 %.4f%%，这次才 %.4f%%？手是不是借给别人了？",
		"退步这么明显，你是开了省电模式在玩吗？成绩从 %.4f%%掉到了 %.4f%%。",
		"恭喜解锁新成就：‘个人最差’！从 %.4f%% 到 %.4f%%，了不起。",
		"你管这叫热身？我看是直接入土。上次 %.4f%%，这次 %.4f%%。",
		"哟，成绩还会往下掉的啊？从 %.4f%% 变成 %.4f%%，我大开眼界。",
		"上次 %.4f%%，这次 %.4f%%。没事，就当是给过去的自己一点面子。",
		"真是精准地停留在了自己的上限啊！（%.4f%%）打个%.4f%%你好意思的？",
		"你这哪是平稳发挥，简直是原地踏步，上次 %.4f%%，这次 %.4f%%。",
		"瓶颈期了是吧，要不要帮你找个教练？成绩还不如上次打的 %.4f%%。",
	}

	// 当分数不变时 newRate == oldScore.AchievementRate
	sameScoreResponses := []string{
		"《%s》的成绩和上次一样（%.4f%%），精准地停留在了自己的上限。",
		"《%s》又是 %.4f%%，你这哪是平稳发挥，简直是原地踏步。",
		"《%s》瓶颈期了是吧，要不要帮你找个教练？成绩一直是 %.4f%%。",
		"《%s》打了 %.4f%%，不多也不少，主打一个陪伴上次的自己。",
	}

	if !scoreExists {
		response = fmt.Sprintf("大神啊！《%s》居然打到了 %.4f%% ！", displayTitle, newRate)
	} else if newRate > oldScore.AchievementRate {
		response = fmt.Sprintf("太强了！《%s》的成绩从 %.4f%% 刷新到了 %.4f%%！", displayTitle, oldScore.AchievementRate, newRate)
	} else if newRate < oldScore.AchievementRate {
		// 随机选择一句讽刺的话
		format := lowerScoreResponses[rand.Intn(len(lowerScoreResponses))]
		response = fmt.Sprintf(format, oldScore.AchievementRate, newRate)
	} else { // newRate == oldScore.AchievementRate
		// 随机选择一句讽刺的话
		format := sameScoreResponses[rand.Intn(len(sameScoreResponses))]
		response = fmt.Sprintf(format, displayTitle, newRate) // 注意这里有些句子需要歌名
		shouldSave = false
	}

	if isFallback {
		response = fmt.Sprintf("（调用国际服数据库找到歌曲）\n%s", response)
	}

	if shouldSave {
		scores[userID][songID] = ScoreEntry{
			AchievementRate: newRate,
			Timestamp:       time.Now().Unix(),
		}
	}

	return response, shouldSave
}