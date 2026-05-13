# Changelog

# Unreleased

## Bug Fixes

- Make `go-search/migrations` register source-stable ordered migration sources so host managers using `go-persistence-bun v0.16.0+` do not hit mixed identity errors.
- Add `migrations.LegacyOrderedSources(...)` for positional marker backfill; shared runtime/search repairs still require the host's full historical graph.
- Fix Postgres provider generated-column migrations by moving text/vector expressions behind immutable helper functions.

## Documentation

- Document stable search source keys/orders, normalized database metadata keys, consumer smoke tests, Postgres integration test configuration, and legacy marker repair guidance.

# [0.7.0](https://github.com/goliatone/go-search/compare/v0.6.0...v0.7.0) - (2026-04-18)

## <!-- 1 -->🐛 Bug Fixes

- Typesense id parsing ([36913ae](https://github.com/goliatone/go-search/commit/36913aebd7ce2c6b234e1ebbc3031cbad8c00a18))  - (goliatone)

## <!-- 13 -->📦 Bumps

- Bump version: v0.7.0 ([2dfe421](https://github.com/goliatone/go-search/commit/2dfe4212a3657e0a679f3a9dcd7c58acbf97cb61))  - (goliatone)

## <!-- 3 -->📚 Documentation

- Update changelog for v0.6.0 ([3d266f7](https://github.com/goliatone/go-search/commit/3d266f7a3de151a7d1b39a734e3fb2fcbc12bba9))  - (goliatone)

# [0.6.0](https://github.com/goliatone/go-search/compare/v0.5.0...v0.6.0) - (2026-04-18)

## <!-- 13 -->📦 Bumps

- Bump version: v0.6.0 ([3f6b31f](https://github.com/goliatone/go-search/commit/3f6b31fd14f2575df0e579c39fb0770fb6b5b043))  - (goliatone)

## <!-- 16 -->➕ Add

- Postgres and typesense migration ([27a3185](https://github.com/goliatone/go-search/commit/27a31859f3b2da9d04283cd021b844d448967934))  - (goliatone)
- Mgirations ([1943acf](https://github.com/goliatone/go-search/commit/1943acfb35fb26f40dcad190063d82148365ec8f))  - (goliatone)
- Internal migration util; ([8bbe8df](https://github.com/goliatone/go-search/commit/8bbe8df52c3fa814cdcf7f600c8655a25e227aac))  - (goliatone)

## <!-- 3 -->📚 Documentation

- Update changelog for v0.5.0 ([d755e37](https://github.com/goliatone/go-search/commit/d755e37d49952c97c19df0a56ed69d2b8ee2afce))  - (goliatone)

## <!-- 7 -->⚙️ Miscellaneous Tasks

- Update docs ([e766078](https://github.com/goliatone/go-search/commit/e766078f8a45f74f97b73ac3ca2c350b6a51ece8))  - (goliatone)
- Update examples ([11f50c7](https://github.com/goliatone/go-search/commit/11f50c77a1d1633f40c65582023a01a9a116f7a3))  - (goliatone)

# [0.5.0](https://github.com/goliatone/go-search/compare/v0.4.1...v0.5.0) - (2026-04-17)

## <!-- 13 -->📦 Bumps

- Bump version: v0.5.0 ([329060c](https://github.com/goliatone/go-search/commit/329060c0c2776016c55a635e45736482a0e556ba))  - (goliatone)

## <!-- 3 -->📚 Documentation

- Update changelog for v0.4.1 ([348d3b9](https://github.com/goliatone/go-search/commit/348d3b92c2ad9569f065912da790f43a21ff8fd7))  - (goliatone)

# [0.4.1](https://github.com/goliatone/go-search/compare/v0.4.0...v0.4.1) - (2026-04-17)

## <!-- 13 -->📦 Bumps

- Bump version: v0.4.1 ([6c2680a](https://github.com/goliatone/go-search/commit/6c2680a2777c90412c05a103b0c40d879973f825))  - (goliatone)

## <!-- 16 -->➕ Add

- Baseline tools ([1252df3](https://github.com/goliatone/go-search/commit/1252df3272f3506924804dc96c27948b44a37e1d))  - (goliatone)
- Go-cms projector ([5b5483a](https://github.com/goliatone/go-search/commit/5b5483a22cd8582976456e7077f44f0a0d51ac35))  - (goliatone)
- Content projector for numeric ([7f0d1d5](https://github.com/goliatone/go-search/commit/7f0d1d5543ca353e5c53ae1d4f3e60da7842cf46))  - (goliatone)

## <!-- 3 -->📚 Documentation

- Update changelog for v0.4.0 ([acd3008](https://github.com/goliatone/go-search/commit/acd3008e5533d1bb0c4e013102761a33fa4f5514))  - (goliatone)

## <!-- 7 -->⚙️ Miscellaneous Tasks

- Quality code linter ([d602047](https://github.com/goliatone/go-search/commit/d60204749be2c7cd87fe52afaf91e8a36b8a3a78))  - (goliatone)
- Update deps ([90adf9f](https://github.com/goliatone/go-search/commit/90adf9fb166a0363b3d949bf04b18d010b4b6674))  - (goliatone)
- Update gitignore ([ce4dc00](https://github.com/goliatone/go-search/commit/ce4dc00bb11dc35d1a5bd7d8ed11fd03977f3d70))  - (goliatone)
- Update tests ([3aca928](https://github.com/goliatone/go-search/commit/3aca9289d2594852dc1b659eab651f442e0306ab))  - (goliatone)

# [0.4.0](https://github.com/goliatone/go-search/compare/v0.3.0...v0.4.0) - (2026-04-07)

## <!-- 13 -->📦 Bumps

- Bump version: v0.4.0 ([bdda4ce](https://github.com/goliatone/go-search/commit/bdda4ce8d8106e05914bce39ab4f7e663a8c3548))  - (goliatone)

## <!-- 16 -->➕ Add

- Jobs to index content ([bcde4cb](https://github.com/goliatone/go-search/commit/bcde4cba27f121eafbf3c643ab94238fe5cd7427))  - (goliatone)
- Job dispatch store ([92bbfeb](https://github.com/goliatone/go-search/commit/92bbfeb65324da2d6c287d6a8b47f3ac5d6897f1))  - (goliatone)

## <!-- 3 -->📚 Documentation

- Update changelog for v0.3.0 ([92666fd](https://github.com/goliatone/go-search/commit/92666fdb46cb0a475d1d0f6f739ac5a65c6f3b93))  - (goliatone)

## <!-- 7 -->⚙️ Miscellaneous Tasks

- Update deps ([bf1c90a](https://github.com/goliatone/go-search/commit/bf1c90a0ea8cac5bcc6f6cf3f4595f131decb575))  - (goliatone)

# [0.3.0](https://github.com/goliatone/go-search/compare/v0.2.0...v0.3.0) - (2026-04-06)

## <!-- 1 -->🐛 Bug Fixes

- Remove unused code ([ff9969f](https://github.com/goliatone/go-search/commit/ff9969fbc44ac2c1627b618f35fc0c92fac9bb0b))  - (goliatone)

## <!-- 13 -->📦 Bumps

- Bump version: v0.3.0 ([ff5e251](https://github.com/goliatone/go-search/commit/ff5e251cb8e0d4839d52dff76692038f0d6c72d4))  - (goliatone)

## <!-- 3 -->📚 Documentation

- Update changelog for v0.2.0 ([fbad2af](https://github.com/goliatone/go-search/commit/fbad2af7ef58444c907627029efa8191f6254b10))  - (goliatone)

## <!-- 7 -->⚙️ Miscellaneous Tasks

- Update deps ([c06fc26](https://github.com/goliatone/go-search/commit/c06fc263ce0df0f55f6e5c50d9b487f94efd672c))  - (goliatone)
- Fix code ([d8aa307](https://github.com/goliatone/go-search/commit/d8aa307e943f707442f825181080bf9b5b4c3050))  - (goliatone)
- Update examples ([78a2c26](https://github.com/goliatone/go-search/commit/78a2c26f9627b8ab2e36a3b385829cf8ac8ebf33))  - (goliatone)

# [0.2.0](https://github.com/goliatone/go-search/compare/v0.1.0...v0.2.0) - (2026-03-24)

## <!-- 1 -->🐛 Bug Fixes

- Code quality and static checker feedback ([bc7fbe0](https://github.com/goliatone/go-search/commit/bc7fbe04cd80c6a95e1e66295686bf206a5f60e7))  - (goliatone)
- Code quality ([ef837bb](https://github.com/goliatone/go-search/commit/ef837bb8fc4837806478d0a600c8d40b6f33fca3))  - (goliatone)

## <!-- 13 -->📦 Bumps

- Bump version: v0.2.0 ([076caac](https://github.com/goliatone/go-search/commit/076caac1b1e31b77e6807fd0fdd634c4532a2304))  - (goliatone)

## <!-- 16 -->➕ Add

- Filter validate ([8833817](https://github.com/goliatone/go-search/commit/88338178043c29880e8af91d69c31c27b839216e))  - (goliatone)
- Cache provider setup ([cc19aed](https://github.com/goliatone/go-search/commit/cc19aed143c08b92f695b3bbc0fa47b9c6252671))  - (goliatone)
- Tools for static analysis ([de59a04](https://github.com/goliatone/go-search/commit/de59a046017cf013d29362e7f7ab37e55d17ba79))  - (goliatone)

## <!-- 3 -->📚 Documentation

- Update changelog for v0.1.0 ([bd014ad](https://github.com/goliatone/go-search/commit/bd014ad88234230a863f6844d73dce1cb0cab916))  - (goliatone)

## <!-- 7 -->⚙️ Miscellaneous Tasks

- Fix code ([99dd0cd](https://github.com/goliatone/go-search/commit/99dd0cde49bcf2081bdb261d436f24067643006d))  - (goliatone)
- Update example ([ddc3dd3](https://github.com/goliatone/go-search/commit/ddc3dd3af544af7722b86b00e92191bbb6ade567))  - (goliatone)
- Update CI/CD setup ([10b69d4](https://github.com/goliatone/go-search/commit/10b69d4b560c6430e1b2c7eb95eaac592607b78a))  - (goliatone)
- Update deps ([cdf033e](https://github.com/goliatone/go-search/commit/cdf033e3e991d16ee4832e1b33605ab5aaa15a9c))  - (goliatone)
- Update tests ([513b472](https://github.com/goliatone/go-search/commit/513b4728fa3ca5aacc43bbdd56ecd547e49eebd3))  - (goliatone)

# [0.1.0](https://github.com/goliatone/go-search/tree/v0.1.0) - (2026-03-23)

## <!-- 13 -->📦 Bumps

- Bump version: v0.1.0 ([fd1d402](https://github.com/goliatone/go-search/commit/fd1d40212fbfd29455ae9f47770b4c74778f5a0d))  - (goliatone)

## <!-- 16 -->➕ Add

- Postgres provider ([79094d0](https://github.com/goliatone/go-search/commit/79094d0a701c42346761aca77da61b001c5ef4d1))  - (goliatone)
- Go-cms and go-users adapter ([ddcff61](https://github.com/goliatone/go-search/commit/ddcff611f850f80e78c7979b925a405ee27b03ee))  - (goliatone)
- Cache implementation ([8778dc8](https://github.com/goliatone/go-search/commit/8778dc859e828e280ea32f38dd70c61b0bf31d01))  - (goliatone)
- Go-cms adapter capability cleanup ([d9e5b37](https://github.com/goliatone/go-search/commit/d9e5b37a6ef77a63f41eec09c966a57956925f7f))  - (goliatone)
- Canonical search adoption ([5c9f07e](https://github.com/goliatone/go-search/commit/5c9f07e5a4cc017b00f5ddf598687c840ecbde93))  - (goliatone)
- Updated errors ([ecb70b5](https://github.com/goliatone/go-search/commit/ecb70b505a8b2eb2d1c7c69023777ddb5b69723a))  - (goliatone)
- Indexing registry source ([80df230](https://github.com/goliatone/go-search/commit/80df2302b9bc55216912eb63c17402f120516243))  - (goliatone)
- Go admin adapter translator ([3dd19dc](https://github.com/goliatone/go-search/commit/3dd19dc03ce2e2b0d3e82c2ecd96762d817f0fd8))  - (goliatone)
- Content adapter ([1be69ef](https://github.com/goliatone/go-search/commit/1be69ef9a7bc5574718df1b7acbdf0d3c7291289))  - (goliatone)
- Batched search and grouped counter ([bd305e9](https://github.com/goliatone/go-search/commit/bd305e9976ae3737832eb1b6263698d8d719e81c))  - (goliatone)
- Support for facets ([933a661](https://github.com/goliatone/go-search/commit/933a66157c9fc3e61a49bfd48feda24ac08b96fc))  - (goliatone)
- Parent numeric handler ([bc2211a](https://github.com/goliatone/go-search/commit/bc2211ad02fc2af83121abebf1bcb4dec50e9dec))  - (goliatone)
- Facet support ([07a9dce](https://github.com/goliatone/go-search/commit/07a9dce0a4db6e0b3fff1f5a0d7fc481d53e2287))  - (goliatone)
- Break down subpackages with go.mod ([923d641](https://github.com/goliatone/go-search/commit/923d64108276d8a22b4413b6b88f9a184aacf040))  - (goliatone)
- Support editorial rules ([6a7da0c](https://github.com/goliatone/go-search/commit/6a7da0c9387b4b25713f0ae61805a0b2887d9f36))  - (goliatone)
- Updated error definitions ([e9c001b](https://github.com/goliatone/go-search/commit/e9c001b395db6402ba6d93f2cdd921815683e1b6))  - (goliatone)
- Logger support ([f4818f9](https://github.com/goliatone/go-search/commit/f4818f94904bfc28ed915b98c7d5cfe50de2295c))  - (goliatone)
- Adapters for media and others ([ba91421](https://github.com/goliatone/go-search/commit/ba91421a8807b8b0bd1dbc99039104f6666dc886))  - (goliatone)
- Observe implementation ([e2fa5c3](https://github.com/goliatone/go-search/commit/e2fa5c3057ba6455941bf1ddf15370c8a23c312f))  - (goliatone)
- Implement typesense provider ([551015f](https://github.com/goliatone/go-search/commit/551015ff98e6a6fa70124a2fa363bddc608b2a59))  - (goliatone)
- Implement new APIs in command services ([3237be5](https://github.com/goliatone/go-search/commit/3237be568643b64b528129ea549438e0d69e6d61))  - (goliatone)
- Locale runtime ([b76e27e](https://github.com/goliatone/go-search/commit/b76e27e26093d8493e03dd46b8df4314d13bc1a1))  - (goliatone)
- Locale plan ([d7dcc7a](https://github.com/goliatone/go-search/commit/d7dcc7ab38450d5e59a9f2390c7877e2f8e88b91))  - (goliatone)
- New types for results, ranking, and scope ([323cfa2](https://github.com/goliatone/go-search/commit/323cfa25f2a7f4604705b793abdc955b8608201a))  - (goliatone)
- Clock package ([452b887](https://github.com/goliatone/go-search/commit/452b887e9c71a093ae332800f5ebf613be1d20d0))  - (goliatone)
- Typesense provider ([d5a3840](https://github.com/goliatone/go-search/commit/d5a38406af1a252ba4cb7c682eac3e73125cf533))  - (goliatone)
- Udpated search queries ([b9ba358](https://github.com/goliatone/go-search/commit/b9ba358007e3eb7931e923bd1cce057706826630))  - (goliatone)
- Indexing subitle merge ([ff71789](https://github.com/goliatone/go-search/commit/ff71789745620b969de8baa04800237fd0c118a4))  - (goliatone)
- Commands to manage business logic ([fe12949](https://github.com/goliatone/go-search/commit/fe12949003190b329a3dc553663ba94ecccea764))  - (goliatone)
- Providers contract ([4219dde](https://github.com/goliatone/go-search/commit/4219dde1dd359a9c108e7ce62034544f79ee5416))  - (goliatone)
- Stores to handle editorial and generation data ([45871d4](https://github.com/goliatone/go-search/commit/45871d4fce83f0ed1c30b118d76e100789ae93be))  - (goliatone)
- Memoery provider ([e2f5e5c](https://github.com/goliatone/go-search/commit/e2f5e5c25b470c697b6f082601f264422e65bd15))  - (goliatone)
- Planner ([9baa064](https://github.com/goliatone/go-search/commit/9baa0647beec8585f74e5400a1d3d0fcd5433973))  - (goliatone)
- Internal packages err and test ([a10b5a4](https://github.com/goliatone/go-search/commit/a10b5a499bda524b9c5c3612d34bcdcfafececae))  - (goliatone)
- Indexing ([901b8a3](https://github.com/goliatone/go-search/commit/901b8a343d056f06e5a89274b43ebb8d44239f14))  - (goliatone)
- Adapters for media source and transcripts ([19f18db](https://github.com/goliatone/go-search/commit/19f18db6844b2e634fcd75b24596ca56041c6ea0))  - (goliatone)
- Ranking policy ([3508da5](https://github.com/goliatone/go-search/commit/3508da56b70d2f5398d1ca6bd4b944250d0d621f))  - (goliatone)
- Initial commit ([6035493](https://github.com/goliatone/go-search/commit/60354931c0eef314ddb69b1c8a9bac8f8292392f))  - (goliatone)

## <!-- 2 -->🚜 Refactor

- Registraion key in delete record ([8bd4228](https://github.com/goliatone/go-search/commit/8bd42283e061d064bf078f0800781457a43139e8))  - (goliatone)

## <!-- 7 -->⚙️ Miscellaneous Tasks

- Update tests ([e9e302f](https://github.com/goliatone/go-search/commit/e9e302f5270a41871e05115daac5e2d2f6585388))  - (goliatone)
- Update examples ([6ea5946](https://github.com/goliatone/go-search/commit/6ea5946caf46b794c8f8ad1cb1a1dcdbe645a97d))  - (goliatone)
- Ignore demo binary ([562cda5](https://github.com/goliatone/go-search/commit/562cda5e6c8a0e61bd2b46e3811adfbb7f5e2418))  - (goliatone)
- Fix code ([c37280e](https://github.com/goliatone/go-search/commit/c37280ec71def55d0e3fb74a4b61dfefbb386863))  - (goliatone)
- Update deps ([8276ea4](https://github.com/goliatone/go-search/commit/8276ea44f551aa7efc2c31320fa69cc26b4c22f4))  - (goliatone)
- Release sub pacakges ([18b284a](https://github.com/goliatone/go-search/commit/18b284a9fd3dc7beb1e4d848f491248593bb446c))  - (goliatone)
