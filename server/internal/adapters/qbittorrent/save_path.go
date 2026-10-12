package qbittorrent

import (
	"context"
	"path"
	"strings"

	"magpie/internal/library"
)

const pathDefaultSavePath = "/api/v2/app/defaultSavePath"

var _ library.DownloadLocation = (*Client)(nil)

// SavePath is where qBittorrent saves Magpie's category, as Magpie sees it: the category's own save path
// (relative ones sit under the default), else the default save path.
func (c *Client) SavePath(ctx context.Context) (string, error) {
	conn, err := c.conn(ctx)
	if err != nil {
		return "", err
	}
	body, err := c.call(ctx, request{path: pathDefaultSavePath})
	if err != nil {
		return "", err
	}
	saveTo := strings.TrimSpace(string(body))
	body, err = c.call(ctx, request{path: pathCategories})
	if err != nil {
		return "", err
	}
	var categories map[string]struct {
		SavePath string `json:"savePath"`
	}
	if err := decode(pathCategories, body, &categories); err != nil {
		return "", err
	}
	if own := categories[conn.Category].SavePath; own != "" {
		if path.IsAbs(own) {
			saveTo = own
		} else {
			saveTo = path.Join(saveTo, own)
		}
	}
	return localPath(saveTo, conn), nil
}

// LocalPath maps a path qBittorrent reported to where Magpie sees it, with the mapping set now.
func (c *Client) LocalPath(ctx context.Context, p string) (string, error) {
	conn, err := c.connection(ctx)
	if err != nil {
		return "", err
	}
	return localPath(p, conn), nil
}
