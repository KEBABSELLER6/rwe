# Changelog

## [0.6.0](https://github.com/KEBABSELLER6/rwe/compare/v0.5.0...v0.6.0) (2026-11-28)


### Features

* log per-phase job timings ([39c2194](https://github.com/KEBABSELLER6/rwe/commit/39c21945c4526c5b06739cf2e31ef5e7cfa8f23b))
* log per-phase job timings ([2ad4fe8](https://github.com/KEBABSELLER6/rwe/commit/2ad4fe8a122b263923ba5a380ca6f9bc42b9b0a7))

## [0.5.0](https://github.com/KEBABSELLER6/rwe/compare/v0.4.0...v0.5.0) (2026-11-28)


### Features

* macOS client binaries and install docs ([6a1be28](https://github.com/KEBABSELLER6/rwe/commit/6a1be28d93c6a20d53559a1241c738059ef0e851))
* macOS client binaries in releases ([45cd3c7](https://github.com/KEBABSELLER6/rwe/commit/45cd3c751e57edd1ad803284cff562abda77fdb7))

## [0.4.0](https://github.com/KEBABSELLER6/rwe/compare/v0.3.0...v0.4.0) (2026-11-28)


### Features

* artifact retention and size limit ([4763a77](https://github.com/KEBABSELLER6/rwe/commit/4763a7709a061bfbbdae6efc5cdba9c093253adf))
* artifact retention and size limit ([2f38324](https://github.com/KEBABSELLER6/rwe/commit/2f383249efd2a6aac47382b892dcdb999c537324))
* bring artifacts back to the client ([065402c](https://github.com/KEBABSELLER6/rwe/commit/065402c65343fb54d4d705a857180bd120a71315))
* bring artifacts back to the client ([b03374b](https://github.com/KEBABSELLER6/rwe/commit/b03374b4238da0ca8a8b8cf5ca473fe83a4f9c72))
* cache cleanup ([387fca8](https://github.com/KEBABSELLER6/rwe/commit/387fca8c627549914e0ed30ba8c9130862410584))
* cache cleanup ([0cc2d2a](https://github.com/KEBABSELLER6/rwe/commit/0cc2d2a584cb58ad364d9fe5ed1be34b6934b369))
* collect job artifacts ([3edd4da](https://github.com/KEBABSELLER6/rwe/commit/3edd4da29cdee81912cffe619bd976537fa729fb))
* collect job artifacts ([83e5e1d](https://github.com/KEBABSELLER6/rwe/commit/83e5e1da97c14a1c3232030f89571a34cfacca84))
* download job artifacts as tar.gz ([dd40086](https://github.com/KEBABSELLER6/rwe/commit/dd4008646c1d4add921660a3dddb5b5273691c8c))
* download job artifacts as tar.gz ([5cc6dec](https://github.com/KEBABSELLER6/rwe/commit/5cc6decc11eb100875cea8924200268d5d20f5a9))
* glob patterns for artifacts ([7910dec](https://github.com/KEBABSELLER6/rwe/commit/7910decfde85aafb3d834a33a20b2703bd812b7d))
* glob patterns for artifacts ([86d5795](https://github.com/KEBABSELLER6/rwe/commit/86d5795206b4f8aef32c8005e90d43d37e1ef603))
* workspace cleanup ([006ef6b](https://github.com/KEBABSELLER6/rwe/commit/006ef6b1313b7cb9147d161e901f925da91fc904))
* workspace cleanup ([339a4a7](https://github.com/KEBABSELLER6/rwe/commit/339a4a772cc0d448b6ff3ddfd996bccf545e4363))


### Bug Fixes

* cover 125,126,127 docker exit codes with inspect for real exit code ([c32692f](https://github.com/KEBABSELLER6/rwe/commit/c32692f7099fd93cb6f286f0805d32ed15128df7))
* keep file modification times ([1d3078c](https://github.com/KEBABSELLER6/rwe/commit/1d3078c13b62d7e95cdab538ca3285039c7ce7de))
* map docker exit to real exit code ([78d7fc2](https://github.com/KEBABSELLER6/rwe/commit/78d7fc2515381c2c54cfcab0358f85ed64ad9292))
* map docker exit to real exit code ([c643b7e](https://github.com/KEBABSELLER6/rwe/commit/c643b7e8f032e1e2b47ac2dde650f1b5cb5c9b99))
* remove stale containers after a restart ([22e3242](https://github.com/KEBABSELLER6/rwe/commit/22e32429f2e1c8231d4eec86da1d0000fd118e35))
* remove stale containers after a restart ([5262eb0](https://github.com/KEBABSELLER6/rwe/commit/5262eb0e6ad946f604e1edc74eb7cabeb8a73913))

## [0.3.0](https://github.com/KEBABSELLER6/rwe/compare/v0.2.0...v0.3.0) (2026-11-27)


### Features

* cache directories kept between jobs ([91adb41](https://github.com/KEBABSELLER6/rwe/commit/91adb4177ee3b23e449b920eb435b3d322af264e))
* cache directories kept between jobs ([a24eaa3](https://github.com/KEBABSELLER6/rwe/commit/a24eaa30660dc9024a1d399c84468ffa183c9487))
* cancel jobs with DELETE /jobs/{id} ([8db4da4](https://github.com/KEBABSELLER6/rwe/commit/8db4da45d9cbb15dd2838ccaca6279b360a4b044))
* cleanup job log ([1d687fd](https://github.com/KEBABSELLER6/rwe/commit/1d687fd03e07cf7a44df36cdad02971d62b83315))
* cleanup job log ([87e1bc6](https://github.com/KEBABSELLER6/rwe/commit/87e1bc6de3cd416430c6b47ee79324307b63f3cd))
* enable file mode on sync ([7797374](https://github.com/KEBABSELLER6/rwe/commit/77973746538eea54a95dfb73e86dfb0db21cbd16))
* enable file mode on sync ([0e869bd](https://github.com/KEBABSELLER6/rwe/commit/0e869bdd1558291d2a267393cb743f241013c53d))
* enforce workspace size limit on sync ([3071fce](https://github.com/KEBABSELLER6/rwe/commit/3071fce9e3ca0cab5962a40ecbfc10900c69f163))
* enforce workspace size limit on sync ([b1d2cd1](https://github.com/KEBABSELLER6/rwe/commit/b1d2cd16421cd56fd74f33dba4b1371aaf282cf1))
* persist jobs with json files ([15e536b](https://github.com/KEBABSELLER6/rwe/commit/15e536bc678d7522f719761b6ae70e4230409788))
* persist jobs with json files ([c2509d7](https://github.com/KEBABSELLER6/rwe/commit/c2509d7c25bcf19c4590993ea91bb1b2e3ce53b7))


### Bug Fixes

* cap long log lines at 64kib ([e81acac](https://github.com/KEBABSELLER6/rwe/commit/e81acacf99c58c11f7ee41738b1f3fb1106451bb))
* cap long log lines at 64kib ([4f66a39](https://github.com/KEBABSELLER6/rwe/commit/4f66a3993293199c37bd8cd53921d522fcc1e5cc))
* global memory limit ([63be544](https://github.com/KEBABSELLER6/rwe/commit/63be5449d2727bff2e943f37b447aa6adb5ca05e))
* global memory limit ([1ccda3b](https://github.com/KEBABSELLER6/rwe/commit/1ccda3b9e190647594c30230725f7d98a53c549c))
* job locking ro dir for cleanup ([52e6fe0](https://github.com/KEBABSELLER6/rwe/commit/52e6fe03df66bfc8b9cab1111aa3ad76919de928))
* job locking ro dir for cleanup ([d221ff3](https://github.com/KEBABSELLER6/rwe/commit/d221ff3185fb86d630f1aaaf4589da73e16ecf80))
* raise upload body cap to the workspace size limit ([1b59bcc](https://github.com/KEBABSELLER6/rwe/commit/1b59bccaf0569465ab385d8dafe5d17518d4bd43))

## [0.2.0](https://github.com/KEBABSELLER6/rwe/compare/v0.1.0...v0.2.0) (2026-11-27)


### Features

* add config for workspace and cli ([45c2de4](https://github.com/KEBABSELLER6/rwe/commit/45c2de40699aadc8dd351f9a9323ef9120906f24))
* add server.env config file ([0d295a5](https://github.com/KEBABSELLER6/rwe/commit/0d295a5aff931f31f819ef8149ed27e5548a50b0))
* blob upload ([7003f1b](https://github.com/KEBABSELLER6/rwe/commit/7003f1bec94d707f23e31945bf307fd3469554e3))
* docker runner + scripts ([be10b36](https://github.com/KEBABSELLER6/rwe/commit/be10b36b5457e579bc28a2c8705eb22d7cc559ca))
* job impl ([f99cf92](https://github.com/KEBABSELLER6/rwe/commit/f99cf92b75abcff143aff863c7c7e2c314cace69))
* mcp + cli impl ([85fae4f](https://github.com/KEBABSELLER6/rwe/commit/85fae4fccc1c83f404d1570009168323f4a4a065))
* workspace create handler ([3490f19](https://github.com/KEBABSELLER6/rwe/commit/3490f19060ff0bf02f62d31ab74cf7c18fdf1207))
* workspace sync ([ae0f4ae](https://github.com/KEBABSELLER6/rwe/commit/ae0f4aedfafa93e2d03cd69c5876c208f33d3746))


### Bug Fixes

* crvw findings ([db97d3c](https://github.com/KEBABSELLER6/rwe/commit/db97d3c6a53e2ec59b53f38c7652882d0fc63923))
