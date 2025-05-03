# TODO

* remove tag logic from email server (should be handled here)
* add tags based on incoming email so we can filter?
* maybe try to support TLS with caddy?
* fix admin interface, cannot delet mappings at the least
* Filter for this input:
```
            "body": "\r\n--Apple-Mail-F71A215D-D721-4EF8-9580-05C522F653E7\r\nContent-Type: text/plain;\r\n\tcharset=us-ascii\r\nContent-Transfer-Encoding: 7bit\r\n\r\nhttps://github.com/OpenAgentPlatform/Dive\r\n\r\n--Apple-Mail-F71A215D-D721-4EF8-9580-05C522F653E7\r\nContent-Type: text/html;\r\n\tcharset=utf-8\r\nContent-Transfer-Encoding: 7bit\r\n\r\n<html><head><meta http-equiv=\"content-type\" content=\"text/html; charset=utf-8\"></head><body dir=\"auto\"><a href=\"https://github.com/OpenAgentPlatform/Dive\">https://github.com/OpenAgentPlatform/Dive</a><div dir=\"ltr\"></div></body></html>\r\n--Apple-Mail-F71A215D-D721-4EF8-9580-05C522F653E7--",
```

## UI

* login with tag mappings
* ability to enter events in ui
* display events
* display logs