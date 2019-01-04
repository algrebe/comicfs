# comicfs

A FUSE filesystem in Go Based on bazil.org/fuse
Treats zip files and cbz files as folders.

I plan on building a comic book server with nginx to serve static content.

The client will construct paths such as /static/some-comic.cbz/01.png that nginx will serve out, unaware that some-comic.cbz is actually a compressed file.
