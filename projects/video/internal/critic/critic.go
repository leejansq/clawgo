/*
 * Critic Agent - 评论家智能体
 * 负责从第三方视角审视脚本质量
 */

package critic

import (
	"context"

	"github.com/leejansq/clawgo/projects/video/internal/manager"
	"github.com/leejansq/clawgo/projects/video/pkg/schema"
)

// Critic 评论家智能体
type Critic struct {
	mgr *manager.Manager
}

// NewCritic 创建评论家智能体
func NewCritic(mgr *manager.Manager) *Critic {
	return &Critic{
		mgr: mgr,
	}
}

// Review 审查脚本
func (c *Critic) Review(ctx context.Context, input *schema.CriticInput) (*schema.CriticOutput, error) {
	return c.mgr.ExecuteCritic(ctx, input)
}
