## status

htex is in an *experimental phase*, its API, CLI, and internals might
change in the future. Its syntax is based on [htmlex](https://github.com/dacap/htmlex),
and old C tool/HTML extension to generate static sites.
The way variables (or possibly macros) are get and set
(e.g. tags like [<!get>](docs/#get-variable) and
[<!set>](docs/#set-variable-value)) are probably to change in the future.</p>

There are some missing parts like:

* some [control flow](https://github.com/dacap/htex/issues/1) elements like [`<!function>`](https://github.com/dacap/htex/issues/3), [`<!if>`](https://github.com/dacap/htex/issues/2), [`<!for>`](https://github.com/dacap/htex/issues/4), etc.
* [a way to access a database](https://github.com/dacap/htex/issues/6)
* handle user [sessions/cookies](https://github.com/dacap/htex/issues/7),
* a real static-blog generator (e.g. jekyll) replacement (list of posts/files, permalinks configuration, tags, categories, etc.).

All this to be designed in a future.
