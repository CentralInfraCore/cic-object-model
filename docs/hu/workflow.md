# Fejlesztői Munkafolyamat

Ez a dokumentum a séma keretrendszerrel való interakció tipikus munkafolyamatait vázolja fel, az első beállítástól egy új kiadás létrehozásáig.

## Első Beállítás

Mielőtt elkezdenéd, győződj meg róla, hogy a következő előfeltételek telepítve vannak a gépeden:
- `docker`
- `docker-compose`
- `make`
- `git`

Kövesd ezeket a lépéseket a projekt inicializálásához a repository klónozása után:

1.  **A Vault Aláíró Ügynök Elindítása:**
    A projekthez szükség van egy futó Vault példányra a kiadási artefaktumok aláírásához. Egy segédszkript biztosított egy ideiglenes, helyi Vault szerver futtatásához fejlesztés céljából.

    ```sh
    # Ezt a projekt gyökeréből kell futtatni egy külön terminálban
    ./tools/vault-sign-agent.sh -k /eleresi/ut/a/kulcsodhoz.pem -c /eleresi/ut/a/certedhez.crt --root-ca-file /eleresi/ut/a/CICRootCA.crt
    ```
    Ez az ügynök a háttérben fog futni.

2.  **Python Függőségek Telepítése:**
    Ez a parancs lefordítja a `requirements.in` fájlt, és telepíti az összes szükséges Python csomagot egy helyi `./p_venv` könyvtárba, amelyet a Docker konténer gyorsítótárként használ.

    ```sh
    make infra.deps
    ```

3.  **Docker Image-ek Építése:**
    Építsd meg a `setup` és `builder` szolgáltatásokhoz szükséges Docker image-eket.

    ```sh
    make build
    ```

4.  **A Fejlesztői Konténer Elindítása:**
    Ez elindítja a `builder` konténert a háttérben.

    ```sh
    make up
    ```

5.  **Git Hook-ok Inicializálása:**
    Ez a szkript beállítja a `commit-msg` Git hookot, amely automatikusan aláírja a commitjaidat a futó Vault ügynök segítségével.

    ```sh
    make repo.init
    ```

A környezeted most már teljesen be van állítva és készen áll a fejlesztésre.

## Napi Fejlesztési Feladatok

Ez a tipikus ciklus, amelyet a sémák módosításakor vagy létrehozásakor követni fogsz.

1.  **Séma Módosítása:**
    Végezd el a kívánt módosításokat egy sémafájlon a `/schemas` könyvtárban.

2.  **Validálás Futtatása:**
    Mielőtt kiadást hoznál létre, elengedhetetlen a módosításaid validálása. A `validate` parancs a fordítót csak validálási módban futtatja.

    ```sh
    make validate
    ```

3.  **Tesztek Futtatása:**
    Annak érdekében, hogy maguk az eszközök is megfelelően működjenek, futtasd a `pytest` tesztcsomagot.

    ```sh
    make test
    ```

4.  **Módosítások Commit-olása:**
    Amikor készen vagy, commit-old a módosításaidat. A `commit-msg` hook automatikusan lefut, csatlakozik a helyi Vault ügynökhöz, és egy aláírási blokkot fűz a commit üzenetedhez.

    ```sh
    git add .
    git commit -m "feat: Séma frissítése új tulajdonságokkal"
    ```

## Kiadás Létrehozása

A kiadás itt nem lefordított artefaktum. Az alany a normatív termék: a
specifikáció, a gépi olvasható sémák, minden konformancia-vektor és minden vele
szállított implementáció (SPEC INV-045).

1.  **A munkafa legyen tiszta, és a kapuk menjenek át.**

    ```sh
    make ci
    ```

2.  **Számold ki az alanyt, és rögzítsd.**

    ```sh
    make release.subject          # kiírja a digestet
    # írd be a project.yaml metadata.buildHash mezőjébe
    make manifest-update
    make release.verify           # megerősíti, hogy a leíró ezt a fát írja le
    ```

3.  **Rendelj külső vizsgálatot erre az alanyra**, és tedd le a rekordot
    `reviews/<subject-digest>.md` néven. A `devel` csak ezután érheti el a
    `main`-t (INV-046); az eljárás és a három megrendelő prompt az
    [`external-review.md`](../external-review.md) fájlban van.

    ```sh
    make review.check
    ```

4.  **Nyisd meg a pull requestet a `main`-be.** A CI ehhez a célághoz futtatja a
    `review.check`-et, tehát olyan fa, amihez nincs review, nem mergelhető.

Ez az oldal korábban a `make release-dependency VERSION=v1.0.0` parancsot írta elő.
Az a target soha nem létezett ebben a repositoryban — a bázissablonból örökölt
szöveg volt —, tehát a dokumentált eljárást el sem lehetett kezdeni.

