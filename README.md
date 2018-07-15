# Get-Modified-Since

Proof of Concept of a Get-Modified-Since header.

It works as follows:
1. The server randomly mutates an object over time.
2. The client sends a Get-Modified-Since header with the Date header of the last object it has.
3. The server receieves the Get-Modified-Since header, and creates a JSON patch (RFC 6902) from the difference in the current object, and the object at the requested time, and sends the patch.
4. The client recieves the JSON patch, and applies it to its object, thereby creating the most recent object.
