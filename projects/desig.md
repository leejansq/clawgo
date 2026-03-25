角色
你是一个资深全栈工程师，精通 会话管理和 AI Agent 编排。

任务
请帮我实现一个“聊天机器人管理系统”，参考当前项目（核心架构类似 OpenClaw，一个主 Agent 管理多个子 Agent 的框架）。项目代码放在projects下， 具体要求如下：

主 Agent（管理者）

接收用户指令，分析任务类型。

根据任务启动一个或多个子 Agent 会话，每个子 Agent 拥有独立的会话 ID 和上下文。

能够查看任意子 Agent 的完整聊天记录（包括历史消息和元数据）。

子 Agent（执行者）

每个子 Agent 是一个独立的对话实例，可以有自己的系统提示、工具集和记忆。

与用户的交互方式为 Web Console（即一个网页聊天界面）。

当主 Agent 启动一个子 Agent 时，系统应自动启动一个 WebSocket 或 HTTP 服务，为该子 Agent 提供独立的 Web Console 页面（可以是动态路由，如 /chat/{sessionId}）。用户通过该页面与子 Agent 对话。

技术选型


前端：简单的 HTML/JS 聊天界面，通过 WebSocket 与后端通信。

存储：使用 SQLite（或 JSON 文件）保存每个子 Agent 的对话记录，以及主 Agent 的元信息。


关键功能细节

主 Agent 启动子 Agent：主 Agent 接收一条指令（例如“创建一个客服机器人处理用户退款问题”），它自动生成一个子 Agent，分配唯一 ID，并返回该子 Agent 的 Web Console 访问 URL。

子 Agent 聊天：用户在 Web Console 发送消息，消息通过 WebSocket 发送到后端，后端调用 LLM（子 Agent 的提示词 + 历史）生成回复，返回并显示，同时保存聊天记录。

主 Agent 查看聊天记录：主 Agent 应提供一个接口（例如 /api/sessions/{sessionId}/messages），供查询某个子 Agent 的所有消息。

会话管理：主 Agent 和子 Agent 的会话完全独立，但子 Agent 的元信息（创建时间、状态等）由主 Agent 统一维护。

输出要求

给出完整的项目结构。

提供所有核心代码文件（关键模块、路由、WebSocket 处理、LLM 调用、存储）。

附带简单的启动说明（如何安装依赖、运行服务、访问 Web Console）。

代码风格清晰，有必要的注释。

额外说明

该系统的设计可参考 当前项目类似OpenClaw 的“主控 + 子代理”模式，但不需要实现其全部功能，重点突出“主 Agent 管理子 Agent”和“为每个子 Agent 独立提供 Web Console”两大特性。

Web Console 的静态页面可以嵌入在 Express 中，每个子 Agent 使用相同的 HTML，通过 URL 参数区分会话 ID。