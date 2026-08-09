# Makefile Súgó

Ez a fájl tartalmazza az összes elérhető `make` parancs és azok funkcióinak teljes listáját.

## Konténer Életciklus

- `make up`: Elindítja a `builder` fejlesztői konténert a háttérben. A konténer futva marad, amíg explicit módon le nem állítják.
- `make down`: Leállít és eltávolít minden, a projekthez tartozó konténert, hálózatot és kötetet.
- `make shell`: Interaktív `bash` shellt nyit a futó `builder` konténeren belül. Ez az elsődleges módja a fejlesztői környezettel való interakciónak.
- `make build`: Megépíti vagy újraépíti a `setup` és `builder` szolgáltatásokhoz tartozó Docker image-eket.

## Fő Fejlesztési Feladatok

- `make validate`: A sémafordítót csak validálási módban futtatja. Ellenőrzi a `/schemas` könyvtárban lévő forrás sémákat a meta-sémában definiált szabályok alapján.
- `make test`: Végrehajtja a Python alapú eszközök `pytest` tesztcsomagját. Ez magában foglalja a fordító unit tesztjeit is.
- `make fmt`: Automatikusan formázza az összes Python kódot a `black` és `isort` eszközökkel a konzisztens kódstílus biztosítása érdekében.
- `make lint`: Ellenőrzi a Python kódot a `ruff` és az összes YAML fájlt a `yamllint` eszközzel a lehetséges hibák és stílusproblémák kiszűrésére.
- `make typecheck`: Statikus típusanalízist futtat a Python kódbázison a `mypy` segítségével.
- `make check`: Egy kényelmi parancs, amely sorban futtatja a `fmt`, `lint` és `typecheck` parancsokat.

## Kiadás és provenance

- `make release.subject`: Kiírja a **kiadás alanyát** — egy digestet minden
  követett fájl fölött, kivéve a `MANIFEST.sha256`-ot és a `project.yaml`-t,
  mert egyik sem fedhető le olyan digesttel, amit ő maga hordoz. Így köti a
  `SPEC.md`-t, a sémákat, minden konformancia-vektort és mindkét implementációt.
- `make release.verify`: Ellenőrzi, hogy a `project.yaml` `buildHash`-e az előtte
  lévő fa alanya. Konténeren kívül fut, csak stdlib-bel, hogy egy harmadik fél
  klónnal és egy Pythonnal ellenőrizhessen egy kiadást.
- `make review.check`: Ellenőrzi, hogy létezik-e külső review-rekord erre a fára
  (INV-046). Nem része a `make ci`-nek, mert kiadást kapuz, nem commitot; a CI a
  `main`-be menő pull requesteken futtatja.
- `make release VERSION=<verzió>`: Az örökölt Vault-aláírási út. **A leírókezelése
  még nem valósítja meg az INV-045-öt** — lásd `docs/spec-defects.md` és a
  `reviews/` alatti audit-rekordot.

E szakasz korábbi változata a `make release-dependency` és a `make release-schema`
parancsokat hirdette. Egyik target sem létezett soha ebben a repositoryban: mindkettő
a bázissablonból örökölt szöveg, és a `make -n` mindkettőre azt adja, hogy „No rule
to make target". Aki ezt az oldalt követte, el sem tudott indulni.

## Repository Beállítása

- `make repo.init`: Beállítja a Git hook-okat ehhez a repository-hoz. Jelenleg a `commit-msg` hookot telepíti, amely automatikusan aláírja a commitokat egy helyi Vault ügynök segítségével. Ezt a parancsot a repository klónozása után egyszer kell futtatni.

## Infrastruktúra és Karbantartás

- `make infra.deps`: (Újra)generálja a `requirements.txt` fájlt a `requirements.in` alapján, és telepíti az összes Python függőséget a helyi `./p_venv` gyorsítótárba. Futtasd ezt a parancsot, miután hozzáadtál vagy eltávolítottál egy függőséget a `requirements.in` fájlban.
- `make infra.coverage`: HTML tesztlefedettségi jelentést generál a `./htmlcov` könyvtárba. Ez részletes képet ad arról, hogy a kód mely részeit fedik le a tesztek.
- `make infra.clean`: Egy takarító parancs, amely leállít minden konténert, eltávolít minden generált fájlt (mint a `./p_venv`, `requirements.txt`), és töröl minden gyorsítótárat és Docker kötetet. Ez hasznos, ha teljesen tiszta állapotból szeretnél indulni.
