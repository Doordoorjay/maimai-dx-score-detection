import os
import io
import json
import asyncio
import base64
import time
from fastapi import FastAPI, UploadFile, File
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
from PIL import Image
import logging
from ultralytics import YOLO
import re
import httpx 
import uvicorn

# --------- 初始化 ---------
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("server")

app = FastAPI()
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)

# --- 配置 Debug 模式和文件夹 ---
DEBUG_MODE = False  # 设置为 True 启用调试保存
DEBUG_OUTPUT_DIR = "debug_outputs"
if DEBUG_MODE:
    os.makedirs(DEBUG_OUTPUT_DIR, exist_ok=True)
    logger.info(f"调试模式已启用，图片将保存到: {DEBUG_OUTPUT_DIR}")
# ----------------------------------------

# --- 配置 LM Studio API 地址 ---
LM_STUDIO_API_URL = "http://127.0.0.1:8087/v1/chat/completions"
logger.info(f"将使用本地 LM Studio API: {LM_STUDIO_API_URL}")

# 加载本地 YOLO 模型
try:
    model = YOLO("data/best.pt")
    logger.info("本地 best.pt 模型加载成功！")
except Exception as e:
    logger.error(f"加载 YOLO 模型失败: {e}")
    model = None

# --------- 数据模型 ---------
class OCRResult(BaseModel):
    song_title: str
    achievement_rate: str
    error: str = ""

# --------- 工具函数 ---------
def image_to_bytes(img: Image.Image) -> bytes:
    """将 PIL.Image 对象转换为 PNG 格式的字节流"""
    buf = io.BytesIO()
    img.save(buf, format="PNG")
    return buf.getvalue()

def parse_llm_json(text: str) -> dict:
    """去掉可能存在的 ```json ``` 并解析 JSON"""
    cleaned = re.sub(r"^```(?:json)?|```$", "", text.strip(), flags=re.MULTILINE)
    try:
        return json.loads(cleaned)
    except json.JSONDecodeError:
        logger.error(f"JSON 解析失败, 原文: {text}")
        return {"song_title": "", "achievement_rate": ""}

async def call_local_lm_ocr(image_bytes: bytes, role_hint: str) -> dict:
    """异步调用本地 LM Studio Vision API, role_hint 可为 'song_title' 或 'achievement_rate'"""
    
    try:
        base64_image = base64.b64encode(image_bytes).decode('utf-8')
        image_url = f"data:image/png;base64,{base64_image}"
    except Exception as e:
        logger.error(f"图片 Base64 编码失败: {e}")
        return {"song_title": "", "achievement_rate": "", "error": "Invalid image bytes"}

    prompt = (
        f"请从图片中精确提取 '{role_hint}'。严格按照以下 JSON 格式返回结果，不要添加任何其他描述或说明, "
        '例如：{"song_title": "提取到的歌曲名", "achievement_rate": "提取到的最终达成率（百分比形式:xx.xxxx%/xxx.xxxx%）"}。'
        f"你只需要填充 '{role_hint}' 字段，另一个字段留空。"
    )
    
    payload = {
        "model": "nanonets-ocr-s",
        "messages": [
            {
                "role": "user",
                "content": [
                    {"type": "text", "text": prompt},
                    {"type": "image_url", "image_url": {"url": image_url}}
                ]
            }
        ],
        "max_tokens": 300,
    }

    try:
        async with httpx.AsyncClient(timeout=60.0) as client:
            response = await client.post(LM_STUDIO_API_URL, json=payload)
            response.raise_for_status()
        
        response_data = response.json()
        logger.info(f"LM Studio 原始返回: {response_data}")

        content = response_data['choices'][0]['message']['content']
        result = parse_llm_json(content)
        return result

    except httpx.RequestError as e:
        logger.error(f"调用 LM Studio API 网络错误: {e}")
        return {"song_title": "", "achievement_rate": "", "error": f"LM Studio API Network Error: {e}"}
    except (KeyError, IndexError) as e:
        logger.error(f"解析 LM Studio 返回的 JSON 格式错误: {e}")
        return {"song_title": "", "achievement_rate": "", "error": f"LM Studio API Response Format Error: {e}"}
    except Exception as e:
        logger.error(f"调用 LM Studio API 时发生未知错误: {e}")
        return {"song_title": "", "achievement_rate": "", "error": f"LM Studio API Unknown Error: {e}"}


# --------- FastAPI 接口 ---------
@app.post("/process")
async def process_image(image: UploadFile = File(...)):
    # 异步读取图片字节流
    img_bytes = await image.read()
    # 将字节流转换为 PIL.Image 对象
    img = Image.open(io.BytesIO(img_bytes))

    # --- [新增 Debug 逻辑] ---
    timestamp = int(time.time() * 1000)
    if DEBUG_MODE:
        # 1. 保存原始图片
        original_path = os.path.join(DEBUG_OUTPUT_DIR, f"{timestamp}_original.png")
        img.save(original_path)
        logger.info(f"Debug: 原始图片已保存到 {original_path}")
    # --------------------------

    detected_classes = []
    crop_imgs = {}
    if model:
        results = model(img)
        boxes = results[0].boxes if hasattr(results[0], "boxes") else []

        for box in boxes:
            cls_id = int(box.cls)
            cls_name = model.names[cls_id]
            detected_classes.append(cls_name)

            x1, y1, x2, y2 = map(int, box.xyxy[0].tolist())
            cropped_img = img.crop((x1, y1, x2, y2))
            crop_imgs[cls_name] = cropped_img
            
            if DEBUG_MODE:
                # 2. 保存裁剪后的图片
                cropped_path = os.path.join(DEBUG_OUTPUT_DIR, f"{timestamp}_{cls_name}.png")
                cropped_img.save(cropped_path)
                logger.info(f"Debug: 裁剪图片 ({cls_name}) 已保存到 {cropped_path}")


    logger.info(f"YOLO 检测到的类别: {detected_classes}")

    final_result = {"song_title": "", "achievement_rate": "", "error": ""}

    if "achievement_rate" in crop_imgs and "song_title" in crop_imgs:
        tasks = [
            call_local_lm_ocr(image_to_bytes(crop_imgs["achievement_rate"]), role_hint="achievement_rate"),
            call_local_lm_ocr(image_to_bytes(crop_imgs["song_title"]), role_hint="song_title")
        ]
        results = await asyncio.gather(*tasks)

        for res in results:
            if res.get("achievement_rate"):
                final_result["achievement_rate"] = res.get("achievement_rate", "")
            if res.get("song_title"):
                final_result["song_title"] = res.get("song_title", "")
            if res.get("error"):
                final_result["error"] += res["error"] + "; "
    else:
        final_result["error"] = "未检测到完整的 achievement_rate 和 song_title，跳过 OCR"

    logger.info(f"返回最终结果: {final_result}")
    return final_result

if __name__ == "__main__":
    uvicorn.run("server:app", host="0.0.0.0", port=5001, reload=True)