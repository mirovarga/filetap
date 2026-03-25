# FileTap

REST API for your files.

Indexes a local directory or GitHub repository and serves
file metadata through a queryable HTTP API with filtering, sorting, and
pagination.

## Installation

Download a binary
from [GitHub Releases](https://github.com/mirovarga/filetap/releases).

Or install with Go:

```
go install github.com/mirovarga/filetap@latest
```

Or build from source:

```
make build
```

## Quick Start

```
filetap local .
# In another terminal:
curl localhost:3000/api/files
```

> FileTap does not watch the filesystem for changes. The file index is built at
> startup; restart the server to pick up new or modified files.

## Usage

### Scan a local directory

```
filetap local [directory] [flags]
```

| Flag          | Default | Description                     |
|---------------|---------|---------------------------------|
| `-d, --depth` | 1       | Recursion depth (0 = unlimited) |

```
filetap local                       # Scan current directory (depth 1)
filetap local ./docs -d 0           # Scan ./docs recursively
```

### Scan a GitHub repository

```
filetap github <owner/repo> [flags]
```

| Flag          | Default         | Description                     |
|---------------|-----------------|---------------------------------|
| `-d, --depth` | 0               | Recursion depth (0 = unlimited) |
| `--ref`       | default branch  | Branch, tag, or commit          |
| `--token`     | `$GITHUB_TOKEN` | Personal access token           |

```
filetap github owner/repo -p 8080           # Scan and serve on port 8080
filetap github owner/repo --ref v1.0        # Scan a specific tag
filetap github https://ghe.co/org/repo      # GitHub Enterprise
```

### Global flags

| Flag            | Default | Description                            |
|-----------------|---------|----------------------------------------|
| `-p, --port`    | 3000    | Server port                            |
| `--cors`        |         | CORS allowed origins (comma-separated) |
| `-v, --verbose` | false   | Enable debug logging                   |

## API

### Endpoints

| Method | Path                    | Description          |
|--------|-------------------------|----------------------|
| GET    | `/api/ping`             | Health check         |
| GET    | `/api/files`            | List files           |
| GET    | `/api/files/{hash}`     | Get file by hash     |
| GET    | `/api/files/{hash}/raw` | Get raw file content |

> The raw endpoint serves local files directly and returns a `307` redirect for
> GitHub sources.

### Fields

Each file has the following fields, all usable for filtering, selection, and
sorting
(except `dirs`, which cannot be sorted):

| Field        | Type      | Description                               |
|--------------|-----------|-------------------------------------------|
| `hash`       | string    | Truncated SHA-256 of the file path        |
| `path`       | string    | Relative file path                        |
| `dirs`       | string[]  | Directory path components                 |
| `name`       | string    | Filename with extension                   |
| `baseName`   | string    | Filename without extension                |
| `ext`        | string    | File extension (without dot)              |
| `size`       | int       | Size in bytes                             |
| `modifiedAt` | timestamp | Last modification time (local files only) |
| `mime`       | string    | Detected MIME type                        |

### Filtering

Filter files using `field[operator]=value` query parameters:

| Operator | Description                      | Example               |
|----------|----------------------------------|-----------------------|
| `eq`     | Equal                            | `ext[eq]=md`          |
| `ne`     | Not equal                        | `ext[ne]=tmp`         |
| `gt`     | Greater than                     | `size[gt]=1024`       |
| `gte`    | Greater than or equal            | `size[gte]=1024`      |
| `lt`     | Less than                        | `size[lt]=1048576`    |
| `lte`    | Less than or equal               | `size[lte]=1048576`   |
| `in`     | In list (comma-separated)        | `ext[in]=md,txt,json` |
| `nin`    | Not in list                      | `ext[nin]=exe,bin`    |
| `all`    | All values present (arrays only) | `dirs[all]=src,lib`   |
| `exists` | Field is non-empty               | `ext[exists]=true`    |
| `match`  | Substring match                  | `name[match]=README`  |
| `glob`   | Glob pattern match               | `name[glob]=*.test.*` |
| `nglob`  | Negated glob match               | `name[nglob]=*.min.*` |

> A bare `field=value` is shorthand for `field[eq]=value`.

### Sorting

Sort results with the `order` parameter. Prefix a field with `-` for descending:

```
order=size              # Ascending by size
order=-size             # Descending by size
order=ext,-size         # By extension, then largest first
```

### Pagination

| Parameter | Default | Max  | Description                 |
|-----------|---------|------|-----------------------------|
| `skip`    | 0       |      | Number of results to skip   |
| `limit`   | 100     | 1000 | Number of results to return |

### Field selection

Return only specific fields with the `select` parameter:

```
select=name,size,ext
```

> The `hash` field is always included.

### Response format

All responses contain `meta` and `data`. `meta` holds pagination info (`skip`,
`limit`,
`total`). `data` holds the result – an array for list endpoints, an object for
single
file endpoints. Each file includes a `links` object with `self` and `raw` URLs,
which is always present even when using `select`.

List response (`/api/files`):

```json
{
  "meta": {
    "skip": 0,
    "limit": 100,
    "total": 42
  },
  "data": [
    {
      "hash": "a1b2c3d4e5f67890",
      "path": "src/main.go",
      "dirs": [
        "src"
      ],
      "name": "main.go",
      "baseName": "main",
      "ext": "go",
      "size": 1234,
      "modifiedAt": "2026-01-15T10:30:00Z",
      "mime": "text/x-go",
      "links": {
        "self": "/api/files/a1b2c3d4e5f67890",
        "raw": "/api/files/a1b2c3d4e5f67890/raw"
      }
    }
  ]
}
```

> Single-file endpoints (`/api/files/{hash}`) return the same shape, with `data`
> as an object instead of an array.

### Error responses

On errors the API returns a JSON object with an `error` key:

```json
{
  "error": {
    "message": "not found",
    "details": {
      "hash": "a1b2c3d4e5f67890"
    }
  }
}
```

> The `details` field is included only when additional context is available.

| Status | Meaning                           |
|--------|-----------------------------------|
| 400    | Invalid filter or query parameter |
| 404    | File not found                    |
| 500    | Internal server error             |

### Examples

```
curl localhost:3000/api/files
curl 'localhost:3000/api/files?ext[eq]=md'
curl 'localhost:3000/api/files?select=name,size&order=-size'
curl 'localhost:3000/api/files?name[match]=README'
curl localhost:3000/api/files/{hash}/raw
```

## License

MIT
