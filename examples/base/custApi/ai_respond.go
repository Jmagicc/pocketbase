package custApi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/baidubce/bce-qianfan-sdk/go/qianfan"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/examples/base/dto"
	"net/http"
	"os"
	"time"
)

func LoadCustomizeRouter(app *pocketbase.PocketBase) {
	_ = os.Setenv("QIANFAN_ACCESS_KEY", "GxRvA8gWAh6ubg1KSCBJLzqM")
	_ = os.Setenv("QIANFAN_SECRET_KEY", "5d0bVsp8ZHWvycqx2eL8lpxAxU6N8CZZ")

	HelloExample(app)
	AiStreamRespond(app)   // AiStreamRespond AI 流式返回
	AiCombineTodoList(app) //  AiCombineTodoList AI 整合文本分成多个代办事项

}

func AiStreamRespond(app *pocketbase.PocketBase) {
	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		_, _ = e.Router.AddRoute(echo.Route{
			Method: http.MethodGet,
			Path:   "/api/chat/stream",
			Handler: func(c echo.Context) error {
				// 1. 解析前端给的参数
				userQuestion := c.QueryParam("question")
				if userQuestion == "" {
					return echo.NewHTTPError(http.StatusBadRequest, "问题不能为空")
				}

				// 2. 创建 prompt，并合成完整的消息
				prompt := "你是一个万事通、友好、专业的AI助手，你可以根据用户提供的信息给出合理的解答。请回答以下问题: "
				fullMessage := prompt + " " + userQuestion

				// 3. 调用 SDK 来进行 chat
				chat := qianfan.NewChatCompletion(
					qianfan.WithModel("ERNIE-4.0-8K"),
				)

				// 创建流式请求
				resp, err := chat.Stream(
					context.TODO(),
					&qianfan.ChatCompletionRequest{
						Messages: []qianfan.ChatCompletionMessage{
							qianfan.ChatCompletionUserMessage(fullMessage),
						},
					},
				)
				if err != nil {
					return echo.NewHTTPError(http.StatusInternalServerError, "AI 服务请求失败")
				}

				// 使用 defer 关闭响应流
				defer resp.Close()

				// 4. 流式返回响应
				for {
					r, err := resp.Recv()
					if err != nil {
						// 捕获流式请求的错误并处理
						return echo.NewHTTPError(http.StatusInternalServerError, "接收数据失败: "+err.Error())
					}

					// 判断是否结束
					if resp.IsEnd {
						break
					}
					responseContent := r.Result
					// 将返回的内容推送到前端
					if err := c.Stream(http.StatusOK, "text/event-stream", bytes.NewReader([]byte(responseContent))); err != nil {
						return echo.NewHTTPError(http.StatusInternalServerError, "流式返回失败")
					}
				}
				return nil
			},
			Middlewares: []echo.MiddlewareFunc{
				apis.RequireAdminAuth(),
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
					Content string `json:"content"`
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

				// 4. 解析 AI 返回的数据
				var todoResponse dto.TodoResponse
				err = json.Unmarshal([]byte(resp.Result), &todoResponse)
				if err != nil {
					return echo.NewHTTPError(http.StatusInternalServerError, "解析 AI 响应失败")
				}

				// 5. 为返回的待办事项列表格式化 createdAt
				todoResponse.CreatedAt = time.Now().Format(time.RFC3339) // 使用 RFC3339 格式化当前时间

				// 6. 返回待办事项列表的 JSON 响应
				return c.JSON(http.StatusOK, todoResponse)
			},
			Middlewares: []echo.MiddlewareFunc{
				apis.RequireAdminAuth(),
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
