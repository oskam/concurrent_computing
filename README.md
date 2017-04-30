# URUCHAMIANIE #

`$ go run main.go "in" f`

Gdzie:
- pierwszy argument to nazwa pliku z opisem sieci,
- drugi argument włącza/wyłącza tryb gadatliwy, Go parsuje string wg. takich zasad, co oznacza, że wszystkie te parametry są akceptowane:

```
switch str {
case "1", "t", "T", "true", "TRUE", "True":
    return true, nil
case "0", "f", "F", "false", "FALSE", "False":
    return false, nil
}
```

# PLIK KONFIGURACYJNY #

W kolejnych liniach pliku tekstowego znajdują się:
- podstawowe dane w formacie `s p r t h`, gdzie:
    - `s`: ilość zwrotnic
    - `p`: ilość peronów
    - `r`: ilość torów
    - `t`: ilość pociągów
    - `h`: ile rzeczywistych _sekund_ trwa godzina symulacji
- `s` opisów kolejnych zwrotnic w formacie `id min`, gdzie:
    - `id`: identyfikator zwrotnicy, indeksując od __0__
    - `min`: minimalny czas użycia zwrotnicy w _minutach_
- `p` opisów kolejnych peronów w formacie `id min from to`, gdzie:
    - `id`: identyfikator peronu
    - `min`: minimalny czas postoju w _minutach_
    - `from`: identyfikator zwrotnicy, od której biegnie peron
    - `to`: identyfikator zwrotnicy, do której biegnie peron
- `r` opisów kolejnych torów w formacie `id len speed from to`, gdzie:
    - `id`: identyfikator toru
    - `len`: długość toru w _km_
    - `speed`: maksymalna prędkość pociągów na torze w _km/h_
    - `from`: identyfikator zwrotnicy, od której biegnie tor
    - `to`: identyfikator zwrotnicy, do której biegnie tor
- `t` __dwulinijkowych__ opisów kolejnych pociągów w formacie:
    - pierwsza linia postaci `id speed cap len`, gdzie:
        - `id`: identyfikator pociągu
        - `speed`: maksymalna prędkość pociągu w _km/h_
        - `cap`: maksymalna pojemność pociągu w _osobach_
        - `len`: długość trasy, tj. liczba zwrotnic w cyklu
    - druga linia zawierająca listę `len` identyfikatorów zwrotnic, tworzących cykl, oddzielonych spacjami


> __UWAGA 1:__ Indeksy zwrotnic muszą być kolejnymi liczbami całkowitymi od __0__, są używane jako indeksy w macierzy.

> __UWAGA 2:__ Program zakłada, że dane są wprowadzone prawidłowo i umożliwiają poprawny przebieg symulacji. Nie są wykonywane żadne testy w czasie wczytywania danych, błędny format spowoduje błąd i zakonczenie programu.

### Przykładowy plik: ###

```
3 2 2 1 30
0 5
1 10
2 7
0 5 0 1
1 5 0 1
0 50 120 1 2
1 60 90 2 0
0 100 250 3
1 0 2
```
