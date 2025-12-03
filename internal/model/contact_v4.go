package model

// CREATE TABLE contact(
// id INTEGER PRIMARY KEY,
// username TEXT,
// local_type INTEGER,
// alias TEXT,
// encrypt_username TEXT,
// flag INTEGER,
// delete_flag INTEGER,
// verify_flag INTEGER,
// remark TEXT,
// remark_quan_pin TEXT,
// remark_pin_yin_initial TEXT,
// nick_name TEXT,
// pin_yin_initial TEXT,
// quan_pin TEXT,
// big_head_url TEXT,
// small_head_url TEXT,
// head_img_md5 TEXT,
// chat_room_notify INTEGER,
// is_in_chat_room INTEGER,
// description TEXT,
// extra_buffer BLOB,
// chat_room_type INTEGER
// )
type ContactV4 struct {
	UserName       string `json:"username"`
	Alias          string `json:"alias"`
	Remark         string `json:"remark"`
	NickName       string `json:"nick_name"`
	LocalType      int    `json:"local_type"` // 2 群聊; 3 群聊成员(非好友); 5,6 企业微信;
	Flag           int    `json:"flag"`       // 位标志，Bit 11 (2048) 表示置顶, Bit 28 (268435456) 表示最小化
	BigHeadUrl     string `json:"big_head_url"`
	SmallHeadUrl   string `json:"small_head_url"`
	HeadImgMd5     string `json:"head_img_md5"`
}

func (c *ContactV4) Wrap() *Contact {
	return &Contact{
		UserName:        c.UserName,
		Alias:           c.Alias,
		Remark:          c.Remark,
		NickName:        c.NickName,
		IsFriend:        c.LocalType != 3,
		IsPinned:        (c.Flag & 2048) != 0,       // Bit 11 表示置顶
		IsMinimized:     (c.Flag & 268435456) != 0,  // Bit 28 表示最小化
		BigHeadImgUrl:   c.BigHeadUrl,
		SmallHeadImgUrl: c.SmallHeadUrl,
		HeadImgMd5:      c.HeadImgMd5,
	}
}
