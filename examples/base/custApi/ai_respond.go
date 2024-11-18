package custApi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/baidubce/bce-qianfan-sdk/go/qianfan"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/examples/base/dto"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

func LoadCustomizeRouter(app *pocketbase.PocketBase) {
	_ = os.Setenv("QIANFAN_ACCESS_KEY", "ALTAKRVQUbTZEsTsFCOzzre5oy")
	_ = os.Setenv("QIANFAN_SECRET_KEY", "b5db4f9d06474528877d4468f91e8201")

	HelloExample(app)
	AiStreamRespond(app)   // AiStreamRespond AI 流式返回
	AiCombineTodoList(app) //  AiCombineTodoList AI 整合文本分成多个代办事项

}

func AiStreamRespond(app *pocketbase.PocketBase) {
	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		_, _ = e.Router.AddRoute(echo.Route{
			Method: http.MethodPost,
			Path:   "/api/chat/stream",
			Handler: func(c echo.Context) error {
				// 1. 解析前端的 JSON 请求体
				var req dto.QuestionRequest
				if err := c.Bind(&req); err != nil || req.Question == "" {
					return echo.NewHTTPError(http.StatusBadRequest, "问题不能为空")
				}

				// 2. 构建用户提示词
				prompt := "你是一个万事通、友好、专业的AI助手，你可以根据用户提供的信息给出合理的解答。请回答以下问题: "
				fullMessage := prompt + req.Question

				// 3. 调用 AI SDK 的流式接口
				chat := qianfan.NewChatCompletion(qianfan.WithModel("ERNIE-4.0-8K"))
				resp, err := chat.Stream(
					context.TODO(),
					&qianfan.ChatCompletionRequest{
						Messages: []qianfan.ChatCompletionMessage{
							qianfan.ChatCompletionUserMessage(fullMessage),
						},
					},
				)
				if err != nil {
					fmt.Println("请求失败:", err)
					return echo.NewHTTPError(http.StatusInternalServerError, "AI 服务请求失败")
				}
				defer resp.Close()

				// 设置响应头以支持流式传输
				c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
				c.Response().Header().Set("Cache-Control", "no-cache")
				c.Response().Header().Set("Connection", "keep-alive")
				c.Response().WriteHeader(http.StatusOK)

				// 4. 创建通道用于流式传输
				chanStream := make(chan string)

				// 开启 goroutine 处理 AI 流式响应
				go func() {
					defer close(chanStream)
					for {
						r, err := resp.Recv()
						if err == io.EOF || resp.IsEnd {
							break
						}
						if err != nil {
							chanStream <- fmt.Sprintf("[Error: %s]", err.Error())
							break
						}
						// 去除数据块末尾的换行符
						processedContent := strings.TrimSpace(r.Result)
						chanStream <- processedContent
					}
				}()

				// 5. 流式发送 AI 响应数据
				for msg := range chanStream {
					// 检查是否是错误消息
					if strings.HasPrefix(msg, "[Error:") {
						return echo.NewHTTPError(http.StatusInternalServerError, msg)
					}
					// 写入处理后的数据块
					if _, err := c.Response().Write([]byte(msg)); err != nil {
						return echo.NewHTTPError(http.StatusInternalServerError, "流式发送失败: "+err.Error())
					}
					c.Response().Flush()
				}

				return nil
			},
			Middlewares: []echo.MiddlewareFunc{
				// 根据需要添加认证中间件，例如: AuthenticationMiddleware
			},
		})
		return nil
	})
}

func AiCombineTodoList(app *pocketbase.PocketBase) {
	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		_, _ = e.Router.AddRoute(echo.Route{
			Method: http.MethodPost,
			Path:   "/api/ai/todolist/combine",
			Handler: func(c echo.Context) error {
				var request struct {
					Question string `json:"question"`
					Content  string `json:"content"`
				}
				if err := c.Bind(&request); err != nil {
					return echo.NewHTTPError(http.StatusBadRequest, "无效的请求参数")
				}

				// 2. 创建 AI prompt 和消息
				prompt := fmt.Sprintf(`请将以下内容整理成最多7个代办事项，每个代办事项包括简短的说明。
				最多分成7个代办事项，要求最后返回一个json，不能返回多余回复。json的模板如下呢:
				{
				"steps": [
						{
						  "id": 1,
						  "content": "1.项目范围",
						  "isCompleted": false   // 默认都给false
						},
						{
						  "id": 2,
						  "content": "2.分配任务",
						  "isCompleted": false
						}
					  ]，...
				}
				内容如下：%s`, request.Content)

				// 3. 调用 AI 接口处理内容
				chat := qianfan.NewChatCompletion(
					qianfan.WithModel("ERNIE-4.0-8K"),
				)

				resp, err := chat.Do(
					context.TODO(),
					&qianfan.ChatCompletionRequest{
						Messages: []qianfan.ChatCompletionMessage{
							qianfan.ChatCompletionUserMessage(prompt),
						},
					},
				)
				if err != nil {
					return echo.NewHTTPError(http.StatusInternalServerError, "AI 服务请求失败")
				}

				// 确保 AI 返回的数据为 JSON 格式
				var todoResponse []dto.TodoStep
				resp.Result = correctJSONFormat(resp.Result)

				// 尝试解析为包含 steps 的对象
				var wrapper struct {
					Steps []dto.TodoStep `json:"steps"`
				}
				if err := json.Unmarshal([]byte(resp.Result), &wrapper); err == nil {
					todoResponse = wrapper.Steps
				} else if err := json.Unmarshal([]byte(resp.Result), &todoResponse); err != nil {
					// 尝试直接解析为数组
					fmt.Printf("解析 AI 响应失败，返回的内容：%s, 错误: %v\n", resp.Result, err)
					todoResponse = getDefaultTodoSteps()
				}

				result := dto.TodoResponse{
					Question:  request.Question,
					Steps:     todoResponse,
					CreatedAt: time.Now().Format(time.RFC3339),
				}

				// 6. 返回待办事项列表的 JSON 响应
				return c.JSON(http.StatusOK, result)
			},
			Middlewares: []echo.MiddlewareFunc{
				//apis.RequireAdminAuth(),
			},
		})
		return nil
	})
}

func HelloExample(app *pocketbase.PocketBase) {
	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		// 定义自定义的 `GET /api/custom/hello` 路由
		e.Router.GET("/api/custom/hello", func(c echo.Context) error {
			return c.JSON(http.StatusOK, map[string]string{
				"message": "Hello, PocketBase!",
			})
		})
		return nil
	})
}

// 辅助函数：尝试修正不规范的 JSON 数据
func correctJSONFormat(data string) string {
	// 简单处理：去掉非 JSON 数据的头尾信息或格式问题
	re := regexp.MustCompile(`(?s)({.*})`)
	match := re.FindString(data)
	if match != "" {
		return match
	}
	// 返回一个空的 JSON 数组作为兜底方案
	return "[]"
}

// 辅助函数：设置默认待办事项步骤，避免空数据情况
func getDefaultTodoSteps() []dto.TodoStep {
	return []dto.TodoStep{
		{ID: 1, Content: "暂无内容", IsCompleted: false},
	}
}
