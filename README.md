# 🌸 Maimai DX Score Detection 🌸

> **基于 YOLOv8 + LM Studio (OCR) 的舞萌 DX 成绩自动识别与管理系统**

本项目是一个专为舞萌 DX 设计的自动化工具，能够通过截图自动识别成绩、记录分数，并根据表现提供“人性化”的互动反馈。

---

## 🌟 核心特性

- **精准图像分割**：利用训练好的 **YOLOv8** 模型，精准定位并裁剪出成绩图中的“歌曲标题”与“达成率”区域。
- **大语言模型 OCR**：对接 **LM Studio** 运行的视觉模型（如 `nanonets-ocr-s`），通过大模型理解复杂的艺术字体。
- **多级匹配算法**：具备“主库精确 -> 备库精确 -> 主库模糊 -> 备库模糊”的四级检索逻辑，极大提高歌名识别率。
- **情绪化交互**：内置丰富的响应系统。面对进步会给予赞美；面对退步或原地踏步，机器人会触发随机的“毒舌”嘲讽。
- **自动分数存档**：自动维护 `player_scores.json`，持久化记录每位用户的历史最高水平。

---

## 1. 🛠️ 环境准备

### 1.1 本地 OCR 服务 (LM Studio)
本插件依赖大模型的视觉能力来识别文字，请确保：
1. 下载并安装 [LM Studio](https://lmstudio.ai/)。
2. 下载 **nanonets-ocr-s** 模型 (或其他支持Vision的模型)。
3. 加载模型并开启本地 Server，将端口设置为 `8087`（或在 `server.py` 中修改 `LM_STUDIO_API_URL`）。

### 1.2 Python 后端部署
1.2.1 **安装依赖**：
   ```bash
   pip install fastapi ultralytics pillow httpx uvicorn
  ```
1.2.2 **准备模型**：确保 backend/data/ 目录下存有 best.pt 权重文件。

1.2.3 **启动服务**:
   ```bash
   python backend/server.py
   ```
   服务默认运行在 http://localhost:5001
   
## 2. 🤖 机器人插件安装 (ZeroBot)

该插件是为 [ZeroBot-Plugin](https://github.com/FloatTech/ZeroBot-Plugin) 框架编写的 Go 插件。

2.1 **目录放置**

将 zbp_plugin 目录下的 maimai_scores 文件夹完整复制到你 ZeroBot 项目源码中的 custom/ 文件夹下。

2.2 **插件注册与编译**

在机器人的入口文件 main.go 中优先级中添加导入，以启用插件逻辑：

```go
	_ "github.com/FloatTech/ZeroBot-Plugin/custom/maimai_scores"  // 舞萌成绩图识别
```

修改完成后，重新参考ZeroBot-Plugin文档的[编译部分](https://github.com/FloatTech/ZeroBot-Plugin?tab=readme-ov-file#b-%E6%9C%AC%E5%9C%B0%E7%BC%96%E8%AF%91%E4%BA%A4%E5%8F%89%E7%BC%96%E8%AF%91)来编译你的机器人项目。

2.3 **数据配置 (重要)**

生成目录：首次运行编译后的机器人，系统会自动在机器人的 data/ 目录下生成相关配置文件夹。

迁移数据库：将本项目 zbp_plugin/data/maimai_scores/ 目录下的所有 .json 文件（包括 songs.json 和 all_songs.json）手动拷贝至机器人运行目录下的 data/maimai_scores/ 文件夹内。

## 3. 🎮 使用方法
**发送截图**：在群聊或私聊中直接发送一张纯净的 Maimai DX 结算成绩图。

**自动识别**：插件会自动识别歌曲并与数据库比对。

**反馈逻辑**：

首次记录：大神啊！《歌名》居然打到了 XX.XXXX%！

分数突破：太强了！成绩刷新到了 XX.XXXX%！

表现不佳：触发随机讽刺（例如：“成绩还会往下掉的啊？我大开眼界。”）。

## 4. 📂 文件结构
```text
├── backend/
│   ├── server.py        # Python 后端，提供 YOLO 分割与视觉 OCR 接口
│   └── data/
│       └── best.pt      # YOLO 权重文件
└── zbp_plugin/          # ZeroBot 插件目录
    ├── maimai_scores/   # Go 插件核心
    │   ├── main.go      # 消息拦截与主流程
    │   ├── logic.go     # 匹配算法与回复文案
    │   └── data.go      # 数据库读写逻辑
    └── data/            # 预设的歌曲数据库
```
## 5. 📷 实现预览
1. 用户侧（QQ群内发送成绩图片）
<img width="389" height="615" alt="image" src="https://github.com/user-attachments/assets/f63c8ccb-2500-433f-bc03-dffda97f0651" />

2. Python后端（YOLO识别切割、发送LM Studio API进行ocr）
<img width="1107" height="253" alt="image" src="https://github.com/user-attachments/assets/f542387e-af20-4188-b6f8-1f91cd829f68" />

3. LM Studio侧（ocr并返回分数和歌名）
<img width="718" height="438" alt="image" src="https://github.com/user-attachments/assets/e7ea0997-44c4-430e-83c3-833b3ec0f1e4" />

4. Go插件侧（接收ocr结果并进行歌名/别名匹配、成绩存档）
<img width="788" height="226" alt="image" src="https://github.com/user-attachments/assets/f80f2dd9-6120-4787-a026-c0a1f9178fb2" />

