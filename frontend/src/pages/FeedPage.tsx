import { useEffect, useState } from 'react'
import { Card, Tabs, Tag, Space, Typography, Button, Empty, message, Row, Col } from 'antd'
import { LikeOutlined, CommentOutlined, EyeOutlined, FireOutlined, StarOutlined, StarFilled } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { request } from '../api/client'
import type { PageResult, Post } from '../types'
import { getIdentity } from '../utils/storage'

type ViewKey = 'latest' | 'hot' | 'favorites'

interface FavoriteToggleResult {
  favorited: boolean
  favoriteCount: number
}

export default function FeedPage() {
  const navigate = useNavigate()
  const [posts, setPosts] = useState<Post[]>([])
  const [featured, setFeatured] = useState<Post[]>([])
  const [loading, setLoading] = useState(false)
  const [view, setView] = useState<ViewKey>('latest')
  const [pendingFavorites, setPendingFavorites] = useState<Set<number>>(new Set())
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const pageSize = 20

  const load = async (mode: ViewKey, targetPage = 1, append = false) => {
    setLoading(true)
    try {
      let items: Post[]
      let countTotal: number
      if (mode === 'latest') {
        const data = await request<PageResult<Post>>('get', '/posts', { page: targetPage, page_size: pageSize })
        items = data.items
        countTotal = data.total
      } else if (mode === 'hot') {
        const data = await request<Post[]>('get', '/posts/hot')
        items = data
        countTotal = data.length
      } else {
        if (!getIdentity()) {
          // 未创建匿名身份时收藏接口需要登录，直接展示空态
          setPosts([])
          setTotal(0)
          setPage(targetPage)
          return
        }
        const data = await request<PageResult<Post>>('get', '/favorites', { page: targetPage, page_size: pageSize })
        items = data.items
        countTotal = data.total
      }
      setPosts((prev) => (append ? [...prev, ...items] : items))
      setTotal(countTotal)
      setPage(targetPage)
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  const loadFeatured = async () => {
    try {
      const data = await request<Post[]>('get', '/posts/featured')
      setFeatured(data)
    } catch {
      // 精选加载失败不影响主列表
    }
  }

  useEffect(() => {
    load(view)
    if (view !== 'favorites') {
      loadFeatured()
    }
  }, [view])

  // 切换匿名身份后重新加载，确保点赞/收藏状态只反映当前身份
  useEffect(() => {
    const reload = () => load(view)
    window.addEventListener('gbtreehole:identity-changed', reload)
    return () => window.removeEventListener('gbtreehole:identity-changed', reload)
  }, [view])

  const like = async (postId: number, e: React.MouseEvent) => {
    e.stopPropagation()
    if (!getIdentity()) {
      message.warning('请先创建匿名身份')
      return
    }
    try {
      await request<{ liked: boolean; likeCount: number }>('post', '/likes/toggle', { targetType: 'post', targetId: postId })
      load(view, page)
    } catch (err) {
      message.error((err as Error).message)
    }
  }

  const favorite = async (postId: number, e: React.MouseEvent) => {
    e.stopPropagation()
    if (!getIdentity()) {
      message.warning('请先创建匿名身份')
      return
    }
    // 防重复点击：请求未返回前忽略后续点击，服务端唯一索引也会兜底去重
    if (pendingFavorites.has(postId)) return
    setPendingFavorites((prev) => new Set(prev).add(postId))
    try {
      const result = await request<FavoriteToggleResult>('post', '/favorites/toggle', { postId })
      // 以服务端返回为准更新按钮状态和数量
      setPosts((prev) =>
        prev
          // 在「我的收藏」里取消收藏后，从列表移除该帖
          .filter((p) => !(view === 'favorites' && p.id === postId && !result.favorited))
          .map((p) =>
            p.id === postId ? { ...p, favorited: result.favorited, favoriteCount: result.favoriteCount } : p,
          ),
      )
      if (view === 'favorites') {
        setTotal((t) => Math.max(0, t - (result.favorited ? 0 : 1)))
      }
    } catch (err) {
      message.error((err as Error).message)
    } finally {
      setPendingFavorites((prev) => {
        const next = new Set(prev)
        next.delete(postId)
        return next
      })
    }
  }

  const renderPost = (post: Post) => (
    <Card
      key={post.id}
      className="post-card"
      hoverable
      onClick={() => navigate(`/posts/${post.id}`)}
      style={{ marginBottom: 16 }}
    >
      <Space align="start" style={{ width: '100%' }}>
        <img src={post.avatar} alt={post.nickname} style={{ width: 40, height: 40, borderRadius: '50%' }} />
        <div style={{ flex: 1 }}>
          <Space direction="vertical" size={4} style={{ width: '100%' }}>
            <Space>
              <Typography.Text strong>{post.nickname}</Typography.Text>
              <Typography.Text type="secondary">{post.createdAt}</Typography.Text>
            </Space>
            {post.title && <Typography.Title level={5} style={{ margin: 0 }}>{post.title}</Typography.Title>}
            <Typography.Paragraph style={{ marginBottom: 8, whiteSpace: 'pre-wrap' }}>{post.content}</Typography.Paragraph>
            {post.images?.length > 0 && (
              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 8 }}>
                {post.images.map((url, idx) => (
                  <img key={idx} src={url} alt={`${post.nickname}-${idx}`} style={{ width: 120, height: 120, objectFit: 'cover', borderRadius: 8 }} />
                ))}
              </div>
            )}
            {post.tags.map((tag) => (
              <Tag key={tag.id} color="blue">{tag.name}</Tag>
            ))}
            <Space size="large">
              <Button size="small" type={post.liked ? 'primary' : 'text'} icon={<LikeOutlined />} onClick={(e) => like(post.id, e)}>
                {post.likeCount}
              </Button>
              <Button
                size="small"
                type={post.favorited ? 'primary' : 'text'}
                danger={post.favorited}
                icon={post.favorited ? <StarFilled /> : <StarOutlined />}
                loading={pendingFavorites.has(post.id)}
                onClick={(e) => favorite(post.id, e)}
              >
                {post.favoriteCount}
              </Button>
              <Typography.Text type="secondary"><CommentOutlined /> {post.commentCount}</Typography.Text>
              <Typography.Text type="secondary"><EyeOutlined /> {post.viewCount}</Typography.Text>
              {post.isFeatured && <Typography.Text type="warning"><StarOutlined /> 精选</Typography.Text>}
            </Space>
          </Space>
        </div>
      </Space>
    </Card>
  )

  const emptyText =
    view === 'favorites'
      ? getIdentity()
        ? '还没有收藏，去首页点亮星星收藏喜欢的帖子吧'
        : '请先创建匿名身份后查看收藏'
      : '还没有帖子，快去发布第一条吧'

  return (
    <div>
      {view !== 'favorites' && featured.length > 0 && (
        <Card title={<span><StarOutlined style={{ color: '#faad14' }} /> 每日精选</span>} style={{ marginBottom: 16 }}>
          <Row gutter={12}>
            {featured.slice(0, 3).map((p) => (
              <Col span={8} key={p.id}>
                <Card size="small" hoverable onClick={() => navigate(`/posts/${p.id}`)}>
                  <Typography.Text strong>{p.nickname}</Typography.Text>
                  <Typography.Paragraph ellipsis={{ rows: 2 }} style={{ marginBottom: 0 }}>{p.content}</Typography.Paragraph>
                  <Typography.Text type="secondary"><FireOutlined /> {p.likeCount} 赞 · {p.commentCount} 评论</Typography.Text>
                </Card>
              </Col>
            ))}
          </Row>
        </Card>
      )}
      <Card>
        <Tabs
          activeKey={view}
          onChange={(key) => setView(key as ViewKey)}
          items={[
            { key: 'latest', label: '最新帖子' },
            { key: 'hot', label: '热度排行' },
            { key: 'favorites', label: '我的收藏' },
          ]}
        />
        {loading ? (
          <Typography.Text>加载中...</Typography.Text>
        ) : posts.length === 0 ? (
          <Empty description={emptyText} />
        ) : (
          <>
            {posts.map(renderPost)}
            {view !== 'hot' && total > posts.length && (
              <div style={{ textAlign: 'center', marginTop: 8 }}>
                <Button onClick={() => load(view, page + 1, true)} loading={loading}>加载更多</Button>
              </div>
            )}
          </>
        )}
      </Card>
    </div>
  )
}
