# xml-to-graph

Convertește reprezentările XML ale grafurilor generate cu ajutorul [acestei utilități](http://info.tm.edu.ro:8088/~ORosu/clasa/11b/graf.jar) în fișiere de intrare pentru problemele date la clasă, în stilul pbinfo.

Descarcă pentru [Windows](https://github.com/tmaxmax/xml-to-graph/releases/download/v0.1.0/xml-to-graph.exe) sau [Linux](https://github.com/tmaxmax/xml-to-graph/releases/download/v0.1.0/xml-to-graph) și apoi adaugă locația executabilului în PATH pentru a-l putea rula de oriunde.

## Utilizare

```sh
$ xml-to-graph -help
Usage of xml-to-graph.exe:
  -i string
        The file to parse the graph data from. Defaults to stdin.
  -o string
        The file to write the output to. Defaults to stdout.
```

Convertește input din STDIN și afișează în STDOUT (util când datele sunt primite de la alt script):

```sh
curl http://example.com/graf.xml | xml-to-graph
```

Convertește dintr-un fișier și scrie în altul:

```sh
xml-to-graph -i graf.xml -o input.in
```
