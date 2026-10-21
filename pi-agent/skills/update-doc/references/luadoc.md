# LuaDoc / EmmyLua

- Use the project's annotation dialect, such as EmmyLua or LuaLS.
- Annotate public functions, table shapes, and aliases where useful.
- Use `---@param` and `---@return` to supply type information.
- Explain optional values, failure modes, and Lua-style error returns.
- Skip trivial local helpers and redundant implementation details.

```lua
---@class User
---@field id string
---@field name string

---Find a user by ID; return nil when absent.
---@param users table<string, User>
---@param id string
---@return User|nil
local function findUser(users, id)
  return users[id]
end
```
