# Marioblog

A lightweight personal blog built with Go and the standard library. Posts, comments, and admin settings are stored as JSON on disk—no database required.

## Features

- **Public site** — Home page with post listings, individual post pages, view counts, and emoji-style reactions (like, love, wow, haha, sad, fire, clap)
- **Comments** — Readers can leave comments on posts
- **Admin panel** — Create, edit, and delete posts; upload cover images and avatar; manage profile and account settings
- **File-based storage** — `data/posts.json`, `data/comments.json`, and `data/admin.json` persist everything between restarts

## Requirements

- [Go](https://go.dev/dl/) 1.26 or later

## Quick start

```bash
git clone <repository-url>
cd marioblog
go run .
```

Open [http://localhost:8080](http://localhost:8080) in your browser.

Admin login: [http://localhost:8080/admin/login](http://localhost:8080/admin/login)

On first run, default admin credentials are written to `data/admin.json`. Change the username and password under **Admin → Settings** before deploying anywhere public.

If there are no posts yet, the server seeds a sample story so the home page is not empty.

## Project structure

```
marioblog/
├── main.go              # HTTP server, handlers, JSON persistence
├── data/                # posts.json, comments.json, admin.json
├── templates/           # HTML templates (public + admin)
├── static/              # CSS and other static assets
└── uploads/
    ├── images/          # Post cover images
    └── avatars/         # Admin avatar
```

## Routes

| Path | Description |
|------|-------------|
| `/` | Home — list of posts |
| `/post/{id}` | Single post with comments and reactions |
| `/comment` | POST — add a comment |
| `/post/react` | POST — add a reaction |
| `/admin/login` | Admin sign-in |
| `/admin/dashboard` | Post management |
| `/admin/create` | New post |
| `/admin/edit/{id}` | Edit post |
| `/admin/delete/{id}` | Delete post |
| `/admin/profile` | Public author profile |
| `/admin/settings` | Username, password, and account details |

Static files are served from `/static/` and uploaded media from `/uploads/`.

## Development

Build a binary:

```bash
go build -o marioblog .
./marioblog
```

Dependencies are managed with Go modules (`github.com/google/uuid` for post IDs).

## Data and backups

All content lives under `data/` and `uploads/`. Back up those directories to preserve posts, comments, images, and admin configuration.

## License

No license file is included yet. Add one if you plan to share or open-source the project.
