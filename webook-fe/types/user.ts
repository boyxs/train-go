// 对应后端 domain.User（GET /user/profile 直接返回，无 Result 包装）
// 字段名 PascalCase — Go struct 无 json tag，默认导出 PascalCase
export interface Profile {
  Id: number;
  Email: string;
  Nickname: string;
  Birthday: string; // RFC3339 时间字符串
  AboutMe: string;
  Phone: string;
  CreatedAt: string;
  UpdatedAt: string;
}

// POST /user/login
export interface LoginReq {
  email: string;
  password: string;
}

// POST /user/register
export interface RegisterReq {
  email: string;
  password: string;
  confirmPassword: string;
}

// POST /user/login_sms
export interface SmsLoginReq {
  phone: string;
  code: string;
}

// POST /user/login_sms/code/send
export interface SendCodeReq {
  phone: string;
}

// POST /user/edit
export interface EditProfileReq {
  nickname: string;
  birthday: number; // Unix 毫秒时间戳
  aboutMe: string;
}
