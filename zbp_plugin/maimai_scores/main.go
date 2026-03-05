// custom/maimai_scores/main.go
package maimai_scores

import (
	"bytes"
	"encoding/json"
	"log"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	"github.com/FloatTech/floatbox/web"
	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

var (
	engine *control.Engine
	// 你的 Python AI 服务的地址 (LM studio)
	aiServiceURL = "http://localhost:5001/process"
)

// AiServiceResult 结构用于解析 Python AI 服务返回的最终结果
type AiServiceResult struct {
	SongTitle       string `json:"song_title"`
	AchievementRate string `json:"achievement_rate"`
	Error           string `json:"error"`
}

func init() {
	engine = control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault:  false,
		Brief:             "舞萌成绩识别",
		Help:              "直接发送舞萌成绩截图即可自动识别。",
		PrivateDataFolder: "maimai_scores",
	})

	// 加载主歌曲数据库
	var err error
	songDB, err = LoadSongDatabase("songs.json")
	if err != nil {
		log.Printf("[Maimai Scores AI] FATAL: 主歌曲数据库(songs.json)加载失败: %v", err)
	} else {
		log.Printf("[Maimai Scores AI] 插件已启动，成功加载 %d 条主歌曲数据。\n", len(songDB))
	}

	// 尝试加载全量歌曲数据库（作为回退）
	allSongsDB, err = LoadSongDatabase("all_songs.json")
	if err != nil {
		log.Printf("[Maimai Scores AI] WARN: 全量歌曲数据库(all_songs.json)加载失败，模糊匹配回退功能将不可用: %v", err)
	} else {
		log.Printf("[Maimai Scores AI] 成功加载 %d 条全量歌曲数据（用于回退）。\n", len(allSongsDB))
	}

	engine.On("message/group").SetBlock(false).Handle(imageHandler)
	engine.On("message/private").SetBlock(false).Handle(imageHandler)
}

func imageHandler(ctx *zero.Ctx) {
	// 启动一个独立的协程来处理耗时的AI任务，防止阻塞其他插件
	go func(ctx *zero.Ctx) {
		if len(ctx.Event.Message) != 1 || ctx.Event.Message[0].Type != "image" {
			return
		}
		imageSeg := ctx.Event.Message[0]
		imageURL, ok := imageSeg.Data["url"]
		if !ok || imageURL == "" {
			return
		}

		log.Println("[Maimai Scores] 捕获到纯图片消息，调用全能AI服务...")

		// 1. 下载图片到内存
		imgBytes, err := web.GetData(imageURL)
		if err != nil {
			log.Println("[Maimai Scores] 下载图片失败:", err)
			return
		}

		// 2. 调用 Python AI 服务
		aiResult, err := callAiService(imgBytes)
		if err != nil {
			log.Println("[Maimai Scores] AI 服务调用失败:", err)
			return // AI服务失败时，不打扰用户
		}

		if aiResult.Error != "" || aiResult.SongTitle == "" || aiResult.AchievementRate == "" {
			log.Println("[Maimai Scores] AI 服务未能识别出完整的成绩信息。")
			return
		}

		log.Printf("[Maimai Scores] AI 服务返回结果: 歌名='%s', 达成率='%s'", aiResult.SongTitle, aiResult.AchievementRate)

		// 3. 数据清洗 & 校验
		// --- MODIFIED: 使用新的歌曲名修正函数 ---
		songID, songTitle, isFallback, err := CorrectSongTitleAndGetID(aiResult.SongTitle)
		if err != nil {
			log.Printf("[Maimai Scores] 歌曲名模糊匹配失败: %v", err)
			return
		}
		finalRate, err := ParseAchievementRate(aiResult.AchievementRate)
		if err != nil {
			log.Printf("[Maimai Scores] 达成率解析失败: %v", err)
			return
		}

		// 4. 最终逻辑 & 回复
		log.Printf("[Maimai Scores] 识别成功: UserID=%d, SongID=%s, MatchedTitle=%s, Rate=%.4f%%", ctx.Event.UserID, songID, songTitle, finalRate)

		userID := strconv.FormatInt(ctx.Event.UserID, 10)
		// 调用新的响应生成函数，传入所需参数
		response, shouldSave := GenerateResponseAndSaveScore(userID, songID, songTitle, isFallback, finalRate)

		ctx.SendChain(message.Reply(ctx.Event.MessageID), message.Text(response))

		if shouldSave {
			allScores, loadErr := LoadPlayerScores()
			if loadErr != nil {
				log.Println("[Maimai Scores] 加载分数用于保存时失败:", loadErr)
				return // 避免覆盖数据
			}

			if _, ok := allScores[userID]; !ok {
				allScores[userID] = make(map[string]ScoreEntry)
			}
			// 使用 songID 作为 map 的 key
			allScores[userID][songID] = ScoreEntry{AchievementRate: finalRate, Timestamp: time.Now().Unix()}

			if saveErr := SavePlayerScores(allScores); saveErr != nil {
				log.Println("[Maimai Scores] 保存分数失败:", saveErr)
			}
		}
	}(ctx)
}

// callAiService 将图片字节发送到 Python 服务并获取最终文字结果
func callAiService(imageBytes []byte) (*AiServiceResult, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("image", "score.jpg")
	if err != nil {
		return nil, err
	}
	_, err = part.Write(imageBytes)
	if err != nil {
		return nil, err
	}
	writer.Close()

	req, err := http.NewRequest("POST", aiServiceURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: time.Second * 20}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result AiServiceResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}