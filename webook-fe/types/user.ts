// 对应后端 domain.User
export interface Profile {
  id: number;
  email: string;
  nickname: string;
  birthday: number;
  aboutMe: string;
  phone: string;
  createdAt: number;
  updatedAt: number;
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
  birthday: number;
  aboutMe: string;
}
