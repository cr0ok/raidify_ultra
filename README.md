# raidify_ultra 

A golang command line utility that generates a WoW Classic (Era, TBC, Forever, etc.) priority spreadsheet using data from your guild's [Raidify](https://raidify.app) integration.  If you're not already using Raidify to manage your guild's raiding endeavors, you might be doing it wrong.  

Raidify already connects to your Discord server, warcraftlogs.com, and the Blizzard API to gather the data needed by ULTRA, so all you need to run this utility is your Raidify API key and optionally a Discord webhook to post ULTRA outputs for maximum transparency.  

## Installing & using raidify_ultra

- Clone this repo, build it yourself with "go build", or download a pre-built binary for your system from the releases page.
- Plug in your Raidify API key (and a Discord webhook, if you want) into the .ini file, and run it.  
- Managing multiple guilds?  You can specify a different config file with --config=someOtherConfig.ini
- Testing and don't want to post to Discord?  Add --discord=false to the command line.
- You can tweak the weights and prio eligibility thresholds in the .ini file.

It will fetch wishlists, loot history, and raid attendance from raidify.app and output a new .csv file you can use to set up your prio lists on raidify.app.  The spreadsheet is sorted by raid zone, item, and character.  Not every item listed here necessarily requires a prio list assignment.  

Remind players that winning items tagged "free" (on Raidify) with a Need roll or any item at all on a Greed roll never counts against them - gear up that team!

---

Below is a guide you can post in your Discord to explain to raiders how it works (adjust to your own guild rules specified in your config.ini and on raidify.app):

---

📢 **How ULTRA Works**

To keep loot fair, transparent, and objective, we use the **ULTRA** algorithm alongside **raidify.app** and **Gargul**. Here is a clear breakdown of how wish lists, priority calculations, and in-game rolls tie together. 

---

🎯 **1. The Basics**

* **Wish Lists:** Add up to 8 wish list items on **raidify.app** during the list unlock period.  Your wish list is the single most important factor in determining your loot destiny. Wish list rank is king.  Put your most-wanted item in the top #1 spot on your wish list, for example.  You can also categorize items as "BiS" (up to 3 items), "Upgrade," or "Minor upgrade" to indicate how important they are to you.  These tags don't affect your score or standing if you win the item, but they help officers sort desires for establishing Prio Lists. I recommend tagging your top 3 items as "BiS."  These items on your wish list are respected unless we clearly indicate something different on a raid signup (for example we're hosting an open pick-up raid):
  * The 3 big trinkets in BWL (Drake Fang Talisman, Rejuvenating Gem, and Neltharion's Tear).
  * Any item in AQ40.
  * Any item in Naxx (except Splinters of Atiesh, all legendary items have a manually constructed prio list described in discord). 

* **6-Week Lock:** Once locked, your list stays fixed for 6 weeks of raiding.  After this, the wish lists will be unlocked for 3-4 days, during which you are free to rebuild/re-order your wish list if you want to.  You won't be able to see WHO has a particular item on their wish list, but you'll be able to see how many people do. Before the next 40-man raid, the lists are locked.

* **ULTRA runs before raid:**: The output from the ULTRA system is a spreadsheet used by officers to establish Prio Lists for contentious items ranked 1-3 on wish lists, and it's posted automatically in this channel, so everyone can see the same thing the officers are using to establish prios.  The spreadsheet lists the ULTRA score for each item/character combination and the contributing factors that were used to calculate that score.  100% of questions about why so-and-so was ranked higher than you can be answered by looking at this automated post. 

* **Contention Only:** ULTRA only intervenes when two or more players want the **same item at the same wish list rank** (Ranks 1–3). If an item isn't contentious, wish list order naturally handles it.  We're not sitting around a corpse for 5 minutes figuring out who wins an item - it's decided as soon as the roll-off countdown expires.  We're spending our night killing things in the face, not arguing or even thinking about loot.

---

🎲 **2. In-Game Loot Order (Gargul)**
When loot rolls off in-game, Gargul sorts rolls based on this strict hierarchy:

1. **Prio List Rank** *(Calculated by ULTRA for contentious items)*

2. **Wish List Rank** *(Your 1–8 ranking on Raidify)*

3. **Need +1**

4. **Greed**

The highest roll in the highest category wins. Note that the grand majority of items won't have a prio list at all, so PUGs are welcome and they'll still be able to win non-contentious items just like in any other raid hosted by an established team.

---

🧮 **3. How ULTRA Calculates Your Score**
When multiple players compete for the same item at the same wish list rank (1-3), ULTRA evaluates 5 key metrics over a 6-month window to break ties.  The percentage indicates that factor's weight in determining the overall ULTRA score.

* (20%) **U – Upgrade History:** Decreases when you win big upgrades (items on your wish list) on a Need roll. All characters associated with you are considered. Winning an item ranked #1 impacts your U score a lot, while an item ranked #8 impacts it the least.  Winning an item NOT on your list impacts it even less, and items tagged "free" don't impact it at all.

* (25%) **L – List Commitment:** Rewards how long an item has sat on your character's wish list without being changed.

* (15%) **T – Time Since Last Big Win:** Increases the longer you’ve gone without winning a mainspec wish list item.

* (20%) **R – Overall Raid Attendance:** Measures your total raid attendance for that instance over the past 6 months or since you've started raiding with the team, whichever is shorter. All characters associated with you.

* (20%) **A – Recent Attendance:** Recent attendance in the past 2 months (considering all your characters). 

> ⚠️ **Attendance Protection:** L and T scores are capped by your overall attendance rate (**R**), preventing players with low attendance from hoarding priority.
> 
> 

Note that the ULTRA score for a character-to-item is only calculated based on data from the raid instance where the item drops.  This means that winning a big upgrade in AQ40, for example, will **not impact your standing to win** an item in Naxx.

Also remember that ULTRA only considers the last 6 months of data.  So if you won the top 3 items on your wish list 5 months and 3 weeks ago, your scores will be maxed out next week.

Q) I thought ULTRA was for thatsmybis.com (TMB)?  
A) Yes, it was, but there were many issues with TMB that made setting it up and using it really difficult.  Raidify is the new/better TMB - it's fully integrated with WCL, Blizzard, Gargul, and ULTRA.  It takes about 5 minutes to set up a new guild, less than 5 minutes a week to manage the loot system, and it requires zero brain cells to execute during raids.  So I've discontinued development of TMB/ULTRA.

Q) But this is still loot council, right?  Won't that cause a lot of drama?
A) No, it isn't.  It's a player-driven wish list system.  The entirety of every decision is controlled by you.  You decide what loot is important to you, when to show up for raids, and when to /roll on that loot.  No one else controls these factors.  In practice we see zero loot drama.  Sure, sometimes people need help understanding the published numbers, but once they do, there's no issue.  For players used to 1-dimensional point-based systems (EPGP, GDKP, etc.), it usually takes them 2-3 weeks of using wish lists to fall in love with the system and never look back.  Players looking to cheat the system, gain an unfair advantage, or exploit other players usually don't like the wish list system and leave, which is fine by us.

---

🛡️ **4. Priority Eligibility**
To be eligible for an ULTRA priority slot, you must meet our minimum  attendance thresholds for overall raids in the instance where the item drops (4), player attendance (75%), and character recent attendance (25%). These criteria are only applicable for building prio lists; wish lists are always respected regardless of attendance.

