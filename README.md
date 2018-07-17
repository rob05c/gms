# Get-Modified-Since

Proof of Concept of an HTTP Get-Modified-Since mechanism, to get a patch of a resource for the changes since a given time (or ETag)

## gmsserver and gmsclient

The `gmsserver` and `gmsclient` implement a Get-Modified-Since header, which is an `HTTP-date`. The client requests a `Get-Modified-Since` with the `Date` header from the last response it got from the server, and the server returns a patch from that date to the current object.

It works as follows:
1. The server randomly mutates an object over time.
2. The client sends a Get-Modified-Since header with the Date header of the last object it has.
3. The server receieves the Get-Modified-Since header, and creates a JSON patch (RFC 6902) from the difference in the current object, and the object at the requested time, and sends the patch.
4. The client recieves the JSON patch, and applies it to its object, thereby creating the most recent object.

## gmsetagserver and gmsetagclient

The `gmsetagserver` and `gmsetagclient` behave identically to `gmsserver` and `gmsclient`, except using an ETag instead of HTTP-date in the `Get-Modified-Since` header.

Note a specification could permit both, as `If-Range` does, since an `HTTP-date` is not quoted and `ETag` must be quoted, so they may be unambigiuously distinguished.

## deltaserver and deltaclient

The `deltaserver` and `deltaclient` implement RFC3229 Delta Encoding in HTTP, with a new instance-manipulation value `jsonpatch` implementing RFC6902 JSON Patch.
