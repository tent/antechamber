# Antechamber

Antechamber is a proxy that prevents mixed content warnings by acting as an SSL
origin for images only available via HTTP.

The design is based on GitHub's [Camo](https://github.com/atmos/camo).

## Differences from Camo

- Written in Go, not CoffeeScript/Node.
- Correctly handles conditional GET headers (`ETag`, `Last-Modified`,
  `If-Not-Modified-Since` and `If-None-Match` headers).
- Doesn't have HMAC URL signing.
- Doesn't have connection/status tracking.
- Checks resolved IPs against a blacklist instead of just the hostname.

## URL Formats

```
/?url=<image-url>
/<hex-encoded-image-url>
```
